package service

import "net/url"

type Starter interface {
	StartWithCancel() (*url.URL, func(), error)
}

type Finder interface {
	Find(s string, v *string) (Starter, bool)
}
