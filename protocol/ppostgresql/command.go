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

package ppostgresql

import "container/list"

const (
	flagExecuteOrErrorResponse = 'E'
	flagDescribeOrDataRow      = 'D'
	flagCloseOrCommandComplete = 'C'
)

const (
	flagQuery                   = 'Q'
	flagParse                   = 'P'
	flagBind                    = 'B'
	flagExecute                 = 'E'
	flagDescribe                = 'D'
	flagSync                    = 'S'
	flagTerminate               = 'X'
	flagSSLRequest              = '?'
	flagPasswordMessage         = 'p'
	flagCancelRequestOrCopyDone = 'c'
	flagFunctionCall            = 'F'
	flagClose                   = 'C'
	flagCopyData                = 'd'
	flagCopyBothResponse        = 'W'
	flagCopyInResponse          = 'G'
	flagCopyOutResponse         = 'H'
)

var clientFlagNames = map[uint8]string{
	flagQuery:                   "Query",
	flagParse:                   "Parse",
	flagBind:                    "Bind",
	flagExecute:                 "Execute",
	flagDescribe:                "Describe",
	flagSync:                    "Sync",
	flagTerminate:               "Terminate",
	flagSSLRequest:              "SSLRequest",
	flagPasswordMessage:         "PasswordMessage",
	flagCancelRequestOrCopyDone: "CancelRequestOrCopyDone",
	flagFunctionCall:            "FunctionCall",
	flagClose:                   "Close",
	flagCopyData:                "CopyData",
	flagCopyBothResponse:        "CopyBothResponse",
	flagCopyInResponse:          "CopyInResponse",
	flagCopyOutResponse:         "CopyOutResponse",
}

const (
	flagRowDescription                  = 'T'
	flagDataRow                         = 'D'
	flagCommandComplete                 = 'C'
	flagErrorResponse                   = 'E'
	flagNoticeResponse                  = 'N'
	flagAuthentication                  = 'R'
	flagBackendKeyData                  = 'K'
	flagParameterDescription            = 't'
	flagPortalSuspended                 = 's'
	flagCloseCompleteOrDescribeResponse = '3'
	flagNoData                          = 'n'
	flagReadyForQuery                   = 'Z'
	flagFunctionCallResponse            = 'V'
	flagParseComplete                   = '1'
	flagBindComplete                    = '2'
	flagEmptyQueryResponse              = 'I'
	flagParameterStatus                 = 'v'
)

var serverFlagNames = map[uint8]string{
	flagRowDescription:                  "RowDescription",
	flagDataRow:                         "DataRow",
	flagCommandComplete:                 "CommandComplete",
	flagErrorResponse:                   "ErrorResponse",
	flagNoticeResponse:                  "NoticeResponse",
	flagAuthentication:                  "Authentication",
	flagBackendKeyData:                  "BackendKeyData",
	flagParameterDescription:            "ParameterDescription",
	flagPortalSuspended:                 "PortalSuspended",
	flagCloseCompleteOrDescribeResponse: "CloseCompleteOrDescribeResponse",
	flagNoData:                          "NoData",
	flagReadyForQuery:                   "ReadyForQuery",
	flagFunctionCallResponse:            "FunctionCallResponse",
	flagParseComplete:                   "ParseComplete",
	flagBindComplete:                    "BindComplete",
	flagEmptyQueryResponse:              "EmptyQueryResponse",
	flagParameterStatus:                 "ParameterStatus",
}

type namedStatement struct {
	name      string
	statement string
}

type namedStatementCache struct {
	size int
	l    *list.List
}

func newNamedStatementCache(size int) *namedStatementCache {
	return &namedStatementCache{
		size: size,
		l:    list.New(),
	}
}

func (c *namedStatementCache) Set(name, statement string) {
	if c.l.Len() >= c.size {
		c.l.Remove(c.l.Front())
	}
	c.l.PushBack(namedStatement{
		name:      name,
		statement: statement,
	})
}

func (c *namedStatementCache) Get(name string) string {
	for e := c.l.Front(); e != nil; e = e.Next() {
		if e.Value.(namedStatement).name == name {
			return e.Value.(namedStatement).statement
		}
	}
	return ""
}
