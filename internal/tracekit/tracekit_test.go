// Copyright 2025 The packetd Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tracekit

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTraceIDFromHTTPHeader(t *testing.T) {
	tests := []struct {
		name        string
		traceParent string
		traceID     string
	}{
		{
			name:        "valid",
			traceParent: "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
			traceID:     "0af7651916cd43dd8448eb211c80319c",
		},
		{
			name:        "invalid traceid",
			traceParent: "00-0af7651916cd43dd8448eb211c80319!-b7ad6b7169203331-01",
		},
		{
			name:        "invalid version",
			traceParent: "02-0af7651916cd43dd8448eb211c80319!-b7ad6b7169203331-01",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header := make(http.Header)
			header.Set(headerTraceParent, tt.traceParent)

			got, _ := TraceIDFromHTTPHeader(header)
			assert.Equal(t, tt.traceID, got.String())
		})
	}
}
