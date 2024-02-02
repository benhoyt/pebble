// Copyright (c) 2014-2020 Canonical Ltd
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

	"github.com/canonical/pebble/internals/overlord/state"
)

func v1AckWarnings(c *Command, r *http.Request, _ *UserState) Response {
	// Do nothing; warnings are now notices and acknowledged client-side.
	return SyncResponse(0)
}

func v1GetWarnings(c *Command, r *http.Request, _ *UserState) Response {
	query := r.URL.Query()
	var all bool
	sel := query.Get("select")
	switch sel {
	case "all":
		all = true
	case "pending", "":
		all = false
	default:
		return statusBadRequest("invalid select parameter: %q", sel)
	}

	st := c.d.overlord.State()
	st.Lock()
	defer st.Unlock()

	var notices []*state.Notice
	if all {
		notices = st.Notices(&state.NoticeFilter{
			Types: []state.NoticeType{state.WarningNotice},
		})
	} else {
		notices = statePendingWarnings(st)
	}

	// Convert notices to legacy warning type.
	warnings := make([]*state.Warning, len(notices))
	for i, notice := range notices {
		warning, err := state.NewWarningFromNotice(notice)
		if err != nil {
			return statusInternalError("%s", err)
		}
		warnings[i] = warning
	}
	return SyncResponse(warnings)
}
