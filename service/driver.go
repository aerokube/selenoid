package service

import (
	"errors"
	"net/url"
)

type Driver struct {
	urls chan *url.URL
}

func NewDriver(nodes []string) (*Driver, error) {
	if len(nodes) == 0 {
		return nil, errors.New("empty node list")
	}
	urls := []*url.URL{}
	for _, node := range nodes {
		u, err := url.Parse(node)
		if err != nil {
			return nil, err
		}
		urls = append(urls, u)
	}
	d := &Driver{make(chan *url.URL, len(urls))}
	for _, u := range urls {
		d.urls <- u
	}
	return d, nil
}

func (d *Driver) StartWithCancel() (*url.URL, func(), error) {
	u := <-d.urls
	return u, func() { d.urls <- u }, nil
}
