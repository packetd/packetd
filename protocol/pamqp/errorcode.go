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

package pamqp

var errCodes = map[uint16]string{
	0:   "OK",
	311: "CONTENT_TOO_LARGE",
	312: "NO_ROUTE",
	313: "NO_CONSUMERS",
	403: "ACCESS_REFUSED",
	404: "NOT_FOUND",
	406: "PRECONDITION_FAILED",
	501: "FRAME_ERROR",
	502: "SYNTAX_ERROR",
	503: "COMMAND_INVALID",
	504: "CHANNEL_ERROR",
	505: "UNEXPECTED_FRAME",
}

func matchErrCode(code uint16) string {
	s, ok := errCodes[code]
	if ok {
		return s
	}
	return "Unknown"
}
