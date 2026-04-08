package version

import (
	"time"
)

var (
	GitCommit    = "dev"
	BuildTime    = "2025-10-02T02:17:00"
	MajorVersion = "1"
	MinorVersion = "1"
)

type Version struct {
	Commit string
	Date   string
}

func (v Version) SemVer() string {
	d, _ := time.Parse("2006-01-02T15:04:05", v.Date)
	return MajorVersion + "." + MinorVersion + "." + d.Format("2006.01.02.150405") + "." + v.Commit
}

func Get() Version {
	return Version{
		Commit: GitCommit,
		Date:   BuildTime,
	}
}
