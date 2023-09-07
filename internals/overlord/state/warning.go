// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (c) 2018 Canonical Ltd
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License version 3 as
 * published by the Free Software Foundation.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

package state

import (
	"fmt"
	"time"
)

var (
	defaultWarningRepeatAfter = time.Hour * 24
	defaultWarningExpireAfter = time.Hour * 24 * 28
)

// Warnf records a warning: if it's the first Warning with this
// message it'll be added (with its firstAdded and lastAdded set to the
// current time), otherwise the existing one will have its lastAdded
// updated.
func (s *State) Warnf(template string, args ...interface{}) {
	var message string
	if len(args) > 0 {
		message = fmt.Sprintf(template, args...)
	} else {
		message = template
	}
	if len(message) > MaxNoticeKeyLength {
		// TODO: truncate in the middle
	}
	s.AddNotice(NoticeWarning, message, nil, defaultWarningRepeatAfter)
	notice := s.notices[uniqueNoticeKey(NoticeWarning, message)]
	notice.expireAfter = defaultWarningExpireAfter
}

// WarningsSummary returns the number of warnings that are ready to be
// shown to the user, and the timestamp of the most recently added
// warning (useful for silencing the warning alerts, and OKing the
// returned warnings).
func (s *State) WarningsSummary() (int, time.Time) {
	s.reading()
	// TODO
	/*
		now := time.Now().UTC()
		var last time.Time

		var n int
		for _, w := range s.warnings {
			if w.ShowAfter(now) {
				n++
				if w.lastAdded.After(last) {
					last = w.lastAdded
				}
			}
		}

		return n, last
	*/
	return 0, time.Time{}
}
