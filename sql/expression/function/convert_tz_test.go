// Copyright 2021 Dolthub, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package function

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dolthub/go-mysql-server/sql"
	"github.com/dolthub/go-mysql-server/sql/expression"
)

var timezones = []string{"MET", "US/Central", "US/Eastern"}

func getLocations(t *testing.T) map[string]*time.Location {
	ret := make(map[string]*time.Location)

	for _, tz := range timezones {
		loc, err := time.LoadLocation(tz)
		require.NoError(t, err)
		ret[tz] = loc
	}

	return ret
}

func TestConvertTz(t *testing.T) {
	locationMap := getLocations(t)

	tests := []struct {
		name           string
		datetime       interface{}
		fromTimeZone   string
		toTimeZone     string
		expectedResult interface{}
	}{
		{
			name:           "Simple timezone conversion",
			datetime:       "2004-01-01 12:00:00",
			fromTimeZone:   "GMT",
			toTimeZone:     "MET",
			expectedResult:  time.Date(2004, 1, 1, 13, 0, 0, 0, locationMap["MET"]),
		},
		{
			name:           "Locations going backwards",
			datetime:       "2004-01-01 12:00:00",
			fromTimeZone:   "US/Eastern",
			toTimeZone:     "US/Central",
			expectedResult:  time.Date(2004, 1, 1, 11, 0, 0, 0, locationMap["US/Central"]),
		},
		{
			name:           "Locations going forward",
			datetime:       "2004-01-01 12:00:00",
			fromTimeZone:   "US/Central",
			toTimeZone:     "US/Eastern",
			expectedResult:  time.Date(2004, 1, 1, 13, 0, 0, 0, locationMap["US/Eastern"]),
		},
		{
			name:           "Simple time shift",
			datetime:       "2004-01-01 12:00:00",
			fromTimeZone:   "+01:00",
			toTimeZone:     "+10:00",
			expectedResult:  time.Date(2004, 1, 1, 21, 0, 0, 0, time.UTC),
		},
		{
			name:           "Simple time shift with minutes",
			datetime:       "2004-01-01 12:00:00",
			fromTimeZone:   "+01:00",
			toTimeZone:     "+10:11",
			expectedResult:  time.Date(2004, 1, 1, 21, 11, 0, 0, time.UTC),
		},
		{
			name:           "Different Time Format",
			datetime:       "20100603121212",
			fromTimeZone:   "+01:00",
			toTimeZone:     "+10:00",
			expectedResult:  time.Date(2010, 6, 3, 21, 12, 12, 0, time.UTC),
		},
		{
			name:           "Bad timezone conversion",
			datetime:       "2004-01-01 12:00:00",
			fromTimeZone:   "GMT",
			toTimeZone:     "HLP",
			expectedResult: nil,
		},
		{
			name:           "Bad Time Returns nils",
			datetime:       "2004-01-01 12:00:00dsa",
			fromTimeZone:   "+01:00",
			toTimeZone:     "+10:00",
			expectedResult: nil,
		},
		{
			name:           "Bad Duration Returns nil",
			datetime:       "2004-01-01 12:00:00",
			fromTimeZone:   "+01:00",
			toTimeZone:     "+10:00:11",
			expectedResult: nil,
		},
		{
			name:           "Negative Duration Returns nil",
			datetime:       "2004-01-01 12:00:00",
			fromTimeZone:   "-01:00",
			toTimeZone:     "+10:11",
			expectedResult: nil,
		},
		{
			name:			"Test With Actual datetime type",
			datetime: 		time.Date(2010, 6, 3, 12, 12, 12, 0, time.UTC),
			fromTimeZone:   "+00:00",
			toTimeZone:     "+10:00",
			expectedResult: time.Date(2010, 6, 3, 22, 12, 12, 0, time.UTC),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fn := NewConvertTz(sql.NewEmptyContext(), expression.NewLiteral(test.datetime, sql.Text), expression.NewLiteral(test.fromTimeZone, sql.Text), expression.NewLiteral(test.toTimeZone, sql.Text))

			res, err := fn.Eval(sql.NewEmptyContext(), sql.Row{})
			require.NoError(t, err)

			assert.Equal(t, test.expectedResult, res)
		})
	}

}
