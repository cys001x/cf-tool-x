package cmd

import (
	"github.com/cys001x/cf-tool-x/client"
)

// GetTag command
func GetTag() (err error) {
	cln := client.Instance
	info := Args.Info
	work := func() error {
		return cln.Tags(info)
	}
	if err = work(); err != nil {
		if err = loginAgain(cln, err); err == nil {
			err = work()
		}
	}
	return
}
