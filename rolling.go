package rolling

import (
	"fmt"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/djherbis/times"
)

type RollingFileAppender struct {
	state *state
	mu    sync.RWMutex
	file  *os.File
}

type Config struct {
	Rotation       Rotation
	Directory      string
	FilenamePrefix string
	FilenameSuffix string
	TimeLocation   *time.Location
	MaxFiles       uint32
	DateFormat     string
}

func New(config Config) (*RollingFileAppender, error) {
	state, err := newState(config)
	if err != nil {
		return nil, err
	}

	now := state.getNow()
	file, err := state.createFile(now)
	if err != nil {
		return nil, err
	}

	a := &RollingFileAppender{
		state: state,
		file:  file,
	}

	return a, nil
}

func (r *RollingFileAppender) refreshFile(now time.Time) {
	if r.state.maxFiles > 0 {
		r.state.prune_old_logs()
	}

	newFile, err := r.state.createFile(now)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.file != nil {
		if err := r.file.Close(); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
		}
	}

	r.file = newFile
}

func (r *RollingFileAppender) Write(p []byte) (n int, err error) {
	now := r.state.getNow()
	if current := r.state.shouldRollover(now); current != nil {
		if r.state.AdvanceDate(now, *current) {
			r.refreshFile(now)
		}
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.file.Write(p)
}

func createFile(directory, filename string) (*os.File, error) {
	name := path.Join(directory, filename)

	return os.OpenFile(name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
}

type state struct {
	logDirectory      string
	logFilenamePrefix string
	logFilenameSuffix string
	maxFiles          uint32
	rotation          Rotation
	dateFormat        string
	timeLocation      *time.Location

	nextDate int64
}

func newState(config Config) (*state, error) {
	s := &state{
		logDirectory:      config.Directory,
		logFilenamePrefix: config.FilenamePrefix,
		logFilenameSuffix: config.FilenameSuffix,
		dateFormat:        config.DateFormat,
		timeLocation:      config.TimeLocation,
		maxFiles:          config.MaxFiles,
		rotation:          config.Rotation,
	}

	if s.timeLocation == nil {
		s.timeLocation = time.UTC
	}

	if len(s.dateFormat) == 0 {
		s.dateFormat = "20060102_15:04:05"
	}

	if len(s.logDirectory) == 0 {
		pwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}

		s.logDirectory = pwd
	}

	if nextDate := s.rotation.NextDate(s.getNow()); nextDate != nil {
		s.nextDate = nextDate.Unix()
	}

	return s, nil
}

func (s *state) getNow() time.Time {
	return time.Now().In(s.timeLocation)
}

func (s *state) prune_old_logs() {
	if s.maxFiles == 0 {
		return
	}

	entries, err := os.ReadDir(s.logDirectory)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to read dir", err.Error())
		return
	}

	type LogEntry struct {
		FullPath string
		Ctime    time.Time
	}

	var files []*LogEntry
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filename := entry.Name()
		if len(s.logFilenamePrefix) > 0 && !strings.HasPrefix(filename, s.logFilenamePrefix) {
			continue
		}

		if len(s.logFilenameSuffix) > 0 && !strings.HasSuffix(filename, s.logFilenameSuffix) {
			continue
		}

		fullPath := path.Join(s.logDirectory, filename)
		t, err := times.Stat(fullPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "failed to read file", err)
			continue
		}

		if !t.HasBirthTime() {
			continue
		}

		files = append(files, &LogEntry{FullPath: fullPath, Ctime: t.BirthTime()})
	}

	if len(files) < int(s.maxFiles) {
		return
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Ctime.Before(files[j].Ctime)
	})

	for i := 0; i < len(files)-int(s.maxFiles)+1; i++ {
		err := os.Remove(files[i].FullPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "failed to remove the log entry", err)
		}
	}
}

func (s *state) createFile(date time.Time) (*os.File, error) {
	var filename = s.joinDate(date)

	return createFile(s.logDirectory, filename)
}

func (s *state) shouldRollover(date time.Time) *time.Time {
	var nextDate = atomic.LoadInt64(&s.nextDate)
	if nextDate == 0 {
		return nil
	}

	nextT := time.Unix(nextDate, 0).In(s.timeLocation)
	if date.Unix() >= nextDate {
		return &nextT
	}

	return nil
}

func (s *state) AdvanceDate(now, current time.Time) bool {
	var nextDate = s.rotation.NextDate(now)
	return atomic.CompareAndSwapInt64(&s.nextDate, current.Unix(), nextDate.Unix())
}

func (s *state) joinDate(date time.Time) string {
	dateStr := date.Format(s.dateFormat)

	switch s.rotation {
	case Never:
		if len(s.logFilenamePrefix) > 0 && len(s.logFilenameSuffix) > 0 {
			return s.logFilenamePrefix + s.logFilenameSuffix
		}
		if len(s.logFilenamePrefix) > 0 {
			return s.logFilenamePrefix
		}
		if len(s.logFilenameSuffix) > 0 {
			return s.logFilenameSuffix
		}

	default:
		if len(s.logFilenamePrefix) > 0 && len(s.logFilenameSuffix) > 0 {
			return s.logFilenamePrefix + dateStr + s.logFilenameSuffix
		}
		if len(s.logFilenamePrefix) > 0 {
			return s.logFilenamePrefix + dateStr
		}
		if len(s.logFilenameSuffix) > 0 {
			return dateStr + s.logFilenameSuffix
		}
	}

	return dateStr
}
