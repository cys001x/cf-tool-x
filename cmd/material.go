package cmd

import (
	"errors"

	"github.com/cys001x/cf-tool-x/client"
	"github.com/cys001x/cf-tool-x/config"
	"github.com/fatih/color"
)

// Material command
func Material() (err error) {
	cln := client.Instance
	info := Args.Info
	if info.ContestID == "" {
		return errors.New("You have to specify the Contest ID")
	}

	err = cln.Material(info, config.Instance.Host)
	if err != nil {
		if err = loginAgain(cln, err); err == nil {
			err = cln.Material(info, config.Instance.Host)
		}
	}
	if err != nil {
		color.Red(err.Error())
	}
	return
}
