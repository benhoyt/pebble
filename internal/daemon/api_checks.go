// Copyright (c) 2021 Canonical Ltd
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License version 3 as
// published by the Free Software Foundation.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package daemon

import (
	"net/http"

	"github.com/canonical/pebble/internal/plan"
	"github.com/canonical/pebble/internal/strutil"
)

type checkInfo struct {
	Name         string `json:"name"`
	Level        string `json:"level,omitempty"`
	Healthy      bool   `json:"healthy"`
	Failures     int    `json:"failures,omitempty"`
	LastError    string `json:"last-error,omitempty"`
	ErrorDetails string `json:"error-details,omitempty"`
}

func v1GetChecks(c *Command, r *http.Request, _ *userState) Response {
	level := plan.CheckLevel(r.URL.Query().Get("level"))
	switch level {
	case plan.UnsetLevel, plan.AliveLevel, plan.ReadyLevel:
	default:
		return statusBadRequest(`level must be "alive" or "ready"`)
	}

	names := r.URL.Query()["names"]

	checkMgr := c.d.overlord.CheckManager()
	checks, err := checkMgr.Checks()
	if err != nil {
		return statusInternalError("%v", err)
	}

	var infos []checkInfo
	for _, check := range checks {
		levelMatch := level == plan.UnsetLevel || level == check.Level
		namesMatch := len(names) == 0 || strutil.ListContains(names, check.Name)
		if levelMatch && namesMatch {
			info := checkInfo{
				Name:         check.Name,
				Level:        string(check.Level),
				Healthy:      check.Healthy,
				Failures:     check.Failures,
				LastError:    check.LastError,
				ErrorDetails: check.ErrorDetails,
			}
			infos = append(infos, info)
		}
	}
	return SyncResponse(infos)
}