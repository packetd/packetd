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

type classMethod struct {
	ClassID  uint16
	MethodID uint16
}

type NamedClassMethod struct {
	Class  string
	Method string
}

const (
	classConnection = 10
	classChannel    = 20
	classExchange   = 40
	classQueue      = 50
	classBasic      = 60
	classTx         = 90
)

var classNames = map[uint16]string{
	classConnection: "Connection",
	classChannel:    "Channel",
	classExchange:   "Exchange",
	classQueue:      "Queue",
	classBasic:      "Basic",
	classTx:         "Tx",
}

var classMethodPairs = map[string]string{
	"Start":    "Start-Ok",
	"Secure":   "Secure-Ok",
	"Tune":     "Tune-Ok",
	"Open":     "Open-Ok",
	"Close":    "Close-Ok",
	"Flow":     "Flow-Ok",
	"Declare":  "Declare-Ok",
	"Delete":   "Delete-Ok",
	"Bind":     "Bind-Ok",
	"Unbind":   "Unbind-Ok",
	"Purge":    "Purge-Ok",
	"Qos":      "Qos-Ok",
	"Consume":  "Consume-Ok",
	"Cancel":   "Cancel-Ok",
	"Get":      "Get-Ok",
	"Recover":  "Recover-Ok",
	"Select":   "Select-Ok",
	"Commit":   "Commit-Ok",
	"Rollback": "Rollback-Ok",
}

var classMethodNeedContentHeader = map[classMethod]struct{}{
	{ClassID: classBasic, MethodID: 40}: {},
	{ClassID: classBasic, MethodID: 50}: {},
	{ClassID: classBasic, MethodID: 60}: {},
	{ClassID: classBasic, MethodID: 71}: {},
}

var classMethods = map[classMethod]string{
	// ConnectionClass (10)
	{ClassID: classConnection, MethodID: 10}: "Start",
	{ClassID: classConnection, MethodID: 11}: "Start-Ok",
	{ClassID: classConnection, MethodID: 20}: "Secure",
	{ClassID: classConnection, MethodID: 21}: "Secure-Ok",
	{ClassID: classConnection, MethodID: 30}: "Tune",
	{ClassID: classConnection, MethodID: 31}: "Tune-Ok",
	{ClassID: classConnection, MethodID: 40}: "Open",
	{ClassID: classConnection, MethodID: 41}: "Open-Ok",
	{ClassID: classConnection, MethodID: 50}: "Close",
	{ClassID: classConnection, MethodID: 51}: "Close-Ok",

	// ChannelClass (20)
	{ClassID: classChannel, MethodID: 10}: "Open",
	{ClassID: classChannel, MethodID: 11}: "Open-Ok",
	{ClassID: classChannel, MethodID: 20}: "Flow",
	{ClassID: classChannel, MethodID: 21}: "Flow-Ok",
	{ClassID: classChannel, MethodID: 40}: "Close",
	{ClassID: classChannel, MethodID: 41}: "Close-Ok",

	// ExchangeClass (40)
	{ClassID: classExchange, MethodID: 10}: "Declare",
	{ClassID: classExchange, MethodID: 11}: "Declare-Ok",
	{ClassID: classExchange, MethodID: 20}: "Delete",
	{ClassID: classExchange, MethodID: 21}: "Delete-Ok",

	// QueueClass (50)
	{ClassID: classQueue, MethodID: 10}: "Declare",
	{ClassID: classQueue, MethodID: 11}: "Declare-Ok",
	{ClassID: classQueue, MethodID: 20}: "Bind",
	{ClassID: classQueue, MethodID: 21}: "Bind-Ok",
	{ClassID: classQueue, MethodID: 30}: "Purge",
	{ClassID: classQueue, MethodID: 31}: "Purge-Ok",
	{ClassID: classQueue, MethodID: 40}: "Delete",
	{ClassID: classQueue, MethodID: 41}: "Delete-Ok",
	{ClassID: classQueue, MethodID: 50}: "Unbind",
	{ClassID: classQueue, MethodID: 51}: "Unbind-Ok",

	// BasicClass (60)
	{ClassID: classBasic, MethodID: 10}:  "Qos",
	{ClassID: classBasic, MethodID: 11}:  "Qos-Ok",
	{ClassID: classBasic, MethodID: 20}:  "Consume",
	{ClassID: classBasic, MethodID: 21}:  "Consume-Ok",
	{ClassID: classBasic, MethodID: 30}:  "Cancel",
	{ClassID: classBasic, MethodID: 31}:  "Cancel-Ok",
	{ClassID: classBasic, MethodID: 40}:  "Publish",
	{ClassID: classBasic, MethodID: 50}:  "Return",
	{ClassID: classBasic, MethodID: 60}:  "Deliver",
	{ClassID: classBasic, MethodID: 70}:  "Get",
	{ClassID: classBasic, MethodID: 71}:  "Get-Ok",
	{ClassID: classBasic, MethodID: 72}:  "Get-Empty",
	{ClassID: classBasic, MethodID: 80}:  "Ack",
	{ClassID: classBasic, MethodID: 90}:  "Reject",
	{ClassID: classBasic, MethodID: 100}: "Recover",
	{ClassID: classBasic, MethodID: 101}: "Recover-Ok",
	{ClassID: classBasic, MethodID: 120}: "Nack",

	// TxClass (90)
	{ClassID: classTx, MethodID: 10}: "Select",
	{ClassID: classTx, MethodID: 11}: "Select-Ok",
	{ClassID: classTx, MethodID: 20}: "Commit",
	{ClassID: classTx, MethodID: 21}: "Commit-Ok",
	{ClassID: classTx, MethodID: 30}: "Rollback",
	{ClassID: classTx, MethodID: 31}: "Rollback-Ok",
}

type op uint8

const (
	opSkipUint8 op = iota
	opSkipUint16
	opSkipUint64
	opSkipShortString
	opQueueName
	opExchangeName
	opRoutingKey
	opErrCode
)

type fieldRequest struct {
	ops []op
}

var fieldRequestMap = map[classMethod]fieldRequest{
	// ConnectionClass (10)
	{ClassID: classConnection, MethodID: 50}: {ops: []op{opErrCode}},

	// ChannelClass (20)
	{ClassID: classChannel, MethodID: 40}: {ops: []op{opErrCode}},

	// ExchangeClass (40)
	{ClassID: classExchange, MethodID: 10}: {ops: []op{opSkipUint16, opExchangeName}},
	{ClassID: classExchange, MethodID: 20}: {ops: []op{opSkipUint16, opExchangeName}},

	// QueueClass (50)
	{ClassID: classQueue, MethodID: 10}: {ops: []op{opSkipUint16, opQueueName}},
	{ClassID: classQueue, MethodID: 11}: {ops: []op{opQueueName}},
	{ClassID: classQueue, MethodID: 20}: {ops: []op{opSkipUint16, opQueueName, opExchangeName, opRoutingKey}},
	{ClassID: classQueue, MethodID: 30}: {ops: []op{opSkipUint16, opQueueName}},
	{ClassID: classQueue, MethodID: 40}: {ops: []op{opSkipUint16, opQueueName}},
	{ClassID: classQueue, MethodID: 50}: {ops: []op{opSkipUint16, opQueueName, opExchangeName, opRoutingKey}},

	// BasicClass (60)
	{ClassID: classBasic, MethodID: 20}: {ops: []op{opSkipUint16, opQueueName}},
	{ClassID: classBasic, MethodID: 40}: {ops: []op{opSkipUint16, opExchangeName, opRoutingKey}},
	{ClassID: classBasic, MethodID: 50}: {ops: []op{opSkipUint16, opSkipShortString, opExchangeName, opRoutingKey}},
	{ClassID: classBasic, MethodID: 60}: {ops: []op{opSkipShortString, opSkipUint64, opSkipUint8, opExchangeName, opRoutingKey}},
	{ClassID: classBasic, MethodID: 70}: {ops: []op{opSkipUint16, opQueueName}},
	{ClassID: classBasic, MethodID: 71}: {ops: []op{opSkipUint64, opSkipUint8, opQueueName}},
}
