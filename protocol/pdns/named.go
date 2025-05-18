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

package pdns

import (
	"strconv"

	"golang.org/x/net/dns/dnsmessage"
)

var classNames = map[dnsmessage.Class]string{
	dnsmessage.ClassINET:   "INET",
	dnsmessage.ClassCSNET:  "CSNET",
	dnsmessage.ClassCHAOS:  "CHAOS",
	dnsmessage.ClassHESIOD: "HESIOD",
	dnsmessage.ClassANY:    "ANY",
}

func matchClassName(class dnsmessage.Class) string {
	v, ok := classNames[class]
	if ok {
		return v
	}
	return strconv.Itoa(int(class))
}

var typeNames = map[dnsmessage.Type]string{
	dnsmessage.TypeA:     "A",
	dnsmessage.TypeAAAA:  "AAAA",
	dnsmessage.TypeCNAME: "CNAME",
	dnsmessage.TypeNS:    "NS",
	dnsmessage.TypeMX:    "MX",
	dnsmessage.TypeSOA:   "SOA",
	dnsmessage.TypeSRV:   "SRV",
	dnsmessage.TypePTR:   "PTR",
	dnsmessage.TypeTXT:   "TXT",
}

func matchTypeName(dt dnsmessage.Type) string {
	v, ok := typeNames[dt]
	if ok {
		return v
	}
	return strconv.Itoa(int(dt))
}

var rcodeNames = map[dnsmessage.RCode]string{
	dnsmessage.RCodeSuccess:        "Success",
	dnsmessage.RCodeFormatError:    "FormatError",
	dnsmessage.RCodeServerFailure:  "ServerFailure",
	dnsmessage.RCodeNameError:      "NameError",
	dnsmessage.RCodeNotImplemented: "NotImplemented",
	dnsmessage.RCodeRefused:        "Refused",
}

func matchRcodeName(code dnsmessage.RCode) string {
	v, ok := rcodeNames[code]
	if ok {
		return v
	}
	return strconv.Itoa(int(code))
}

var opCodeNames = map[dnsmessage.OpCode]string{
	dnsmessage.OpCode(0): "Query",
	dnsmessage.OpCode(1): "IQuery",
	dnsmessage.OpCode(2): "Status",
}

func matchOpCodeName(code dnsmessage.OpCode) string {
	v, ok := opCodeNames[code]
	if ok {
		return v
	}
	return strconv.Itoa(int(code))
}
