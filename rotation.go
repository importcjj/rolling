package rolling

import "time"

type Rotation interface {
	NextDate(current time.Time) *time.Time
}

var (
	Never    = newRotation(0)
	Minutely = newRotation(1)
	Hourly   = newRotation(2)
	Daily    = newRotation(3)
)

type RotationKind int8

type rotation struct {
	kind RotationKind
}

func newRotation(kind RotationKind) Rotation {
	return rotation{kind}
}

func (r rotation) NextDate(current time.Time) *time.Time {
	var date time.Time
	switch r.kind {
	case 1:
		date = current.Add(time.Minute)
	case 2:
		date = current.Add(time.Hour)
	case 3:
		date = current.AddDate(0, 0, 1)
	default:
		return nil
	}

	roundDate := r.roundDate(date)
	return &roundDate
}

func (r rotation) roundDate(date time.Time) time.Time {

	switch r.kind {
	case 1:
		return time.Date(date.Year(), date.Month(), date.Day(), date.Hour(), date.Minute(), 0, 0, date.Location())
	case 2:
		return time.Date(date.Year(), date.Month(), date.Day(), date.Hour(), 0, 0, 0, date.Location())
	case 3:
		return time.Date(date.Year(), date.Month(), date.Day(), date.Hour(), 0, 0, 0, date.Location())
	}
	panic("unreachable")
}
