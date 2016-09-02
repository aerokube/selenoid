package service

import "net/url"

type Docker struct{}

func (*Docker) StartWithCancel() (*url.URL, func(), error) {
	u, err := url.Parse("http://localhost:5555")
	if err != nil {
		return nil, nil, err
	}
	return u, func() {}, nil
}
