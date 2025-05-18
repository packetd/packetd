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

package pkafka

// apiKey 是 kafka 协议的重要概念
//
// Kafka API Key 是 Kafka 协议中用于标识不同请求类型的唯一数字标识符
// 每个 Kafka 请求（如生产消息、消费消息、管理元数据等）都对应一个特定的 API Key
// 客户端通过该 Key 告知 Broker 请求的类型 Broker 根据 Key 解析请求并返回响应
// https://kafka.apache.org/protocol#protocol_api_keys
type apiKey uint16

const (
	apiProduce                      apiKey = 0  // 生产消息请求
	apiFetch                        apiKey = 1  // 消费消息请求
	apiListOffsets                  apiKey = 2  // 获取偏移量请求
	apiMetadata                     apiKey = 3  // 元数据请求
	apiLeaderAndIsr                 apiKey = 4  // Leader 和 ISR 变更请求 (内部使用)
	apiStopReplica                  apiKey = 5  // 停止副本请求 (内部使用)
	apiUpdateMetadata               apiKey = 6  // 更新元数据请求 (内部使用)
	apiControlledShutdown           apiKey = 7  // 受控关闭请求 (内部使用)
	apiOffsetCommit                 apiKey = 8  // 提交偏移量请求
	apiOffsetFetch                  apiKey = 9  // 获取偏移量请求
	apiFindCoordinator              apiKey = 10 // 查找协调器请求
	apiJoinGroup                    apiKey = 11 // 加入消费者组请求
	apiHeartbeat                    apiKey = 12 // 心跳请求
	apiLeaveGroup                   apiKey = 13 // 离开消费者组请求
	apiSyncGroup                    apiKey = 14 // 同步消费者组请求
	apiDescribeGroups               apiKey = 15 // 描述消费者组请求
	apiListGroups                   apiKey = 16 // 列出消费者组请求
	apiSaslHandshake                apiKey = 17 // SASL 握手请求
	apiApiVersions                  apiKey = 18 // API 版本请求
	apiCreateTopics                 apiKey = 19 // 创建主题请求
	apiDeleteTopics                 apiKey = 20 // 删除主题请求
	apiDeleteRecords                apiKey = 21 // 删除记录请求
	apiInitProducerId               apiKey = 22 // 初始化生产者 ID 请求
	apiOffsetForLeaderEpoch         apiKey = 23 // 获取 Leader 纪元偏移量请求
	apiAddPartitionsToTxn           apiKey = 24 // 添加分区到事务请求
	apiAddOffsetsToTxn              apiKey = 25 // 添加偏移量到事务请求
	apiEndTxn                       apiKey = 26 // 结束事务请求
	apiWriteTxnMarkers              apiKey = 27 // 写入事务标记请求 (内部使用)
	apiTxnOffsetCommit              apiKey = 28 // 事务偏移量提交请求
	apiDescribeAcls                 apiKey = 29 // 描述 ACL 请求
	apiCreateAcls                   apiKey = 30 // 创建 ACL 请求
	apiDeleteAcls                   apiKey = 31 // 删除 ACL 请求
	apiDescribeConfigs              apiKey = 32 // 描述配置请求
	apiAlterConfigs                 apiKey = 33 // 修改配置请求
	apiAlterReplicaLogDirs          apiKey = 34 // 修改副本日志目录请求
	apiDescribeLogDirs              apiKey = 35 // 描述日志目录请求
	apiSaslAuthenticate             apiKey = 36 // SASL 认证请求
	apiCreatePartitions             apiKey = 37 // 创建分区请求
	apiCreateDelegationToken        apiKey = 38 // 创建委托令牌请求
	apiRenewDelegationToken         apiKey = 39 // 续订委托令牌请求
	apiExpireDelegationToken        apiKey = 40 // 过期委托令牌请求
	apiDescribeDelegationToken      apiKey = 41 // 描述委托令牌请求
	apiDeleteGroups                 apiKey = 42 // 删除消费者组请求
	apiElectLeaders                 apiKey = 43 // 选举 Leader 请求
	apiIncrementalAlterConfigs      apiKey = 44 // 增量修改配置请求
	apiAlterPartitionReassignments  apiKey = 45 // 修改分区重分配请求
	apiListPartitionReassignments   apiKey = 46 // 列出分区重分配请求
	apiOffsetDelete                 apiKey = 47 // 删除偏移量请求
	apiDescribeClientQuotas         apiKey = 48 // 描述客户端配额请求
	apiAlterClientQuotas            apiKey = 49 // 修改客户端配额请求
	apiDescribeUserScramCredentials apiKey = 50 // 描述用户 SCRAM 凭证请求
	apiAlterUserScramCredentials    apiKey = 51 // 修改用户 SCRAM 凭证请求
	apiVote                         apiKey = 52 // 投票请求 (KIP-595)
	apiBeginQuorumEpoch             apiKey = 53 // 开始仲裁纪元请求 (KIP-595)
	apiEndQuorumEpoch               apiKey = 54 // 结束仲裁纪元请求 (KIP-595)
	apiDescribeQuorum               apiKey = 55 // 描述仲裁请求 (KIP-595)
	apiAlterPartition               apiKey = 56 // 修改分区请求
	apiUpdateFeatures               apiKey = 57 // 更新特性请求
	apiEnvelope                     apiKey = 58 // 信封请求
	apiDescribeCluster              apiKey = 60 // 描述集群请求
	apiDescribeProducers            apiKey = 61 // 描述生产者请求
	apiDescribeTransactions         apiKey = 65 // 描述事务请求
	apiListTransactions             apiKey = 66 // 列出事务请求
	apiAllocateProducerIds          apiKey = 67 // 分配生产者 ID 请求
)

var apiKeys = map[apiKey]string{
	apiProduce:                      "Produce",
	apiFetch:                        "Fetch",
	apiListOffsets:                  "ListOffsets",
	apiMetadata:                     "Metadata",
	apiLeaderAndIsr:                 "LeaderAndIsr",
	apiStopReplica:                  "StopReplica",
	apiUpdateMetadata:               "UpdateMetadata",
	apiControlledShutdown:           "ControlledShutdown",
	apiOffsetCommit:                 "OffsetCommit",
	apiOffsetFetch:                  "OffsetFetch",
	apiFindCoordinator:              "FindCoordinator",
	apiJoinGroup:                    "JoinGroup",
	apiHeartbeat:                    "Heartbeat",
	apiLeaveGroup:                   "LeaveGroup",
	apiSyncGroup:                    "SyncGroup",
	apiDescribeGroups:               "DescribeGroups",
	apiListGroups:                   "ListGroups",
	apiSaslHandshake:                "SaslHandshake",
	apiApiVersions:                  "ApiVersions",
	apiCreateTopics:                 "CreateTopics",
	apiDeleteTopics:                 "DeleteTopics",
	apiDeleteRecords:                "DeleteRecords",
	apiInitProducerId:               "InitProducerId",
	apiOffsetForLeaderEpoch:         "OffsetForLeaderEpoch",
	apiAddPartitionsToTxn:           "AddPartitionsToTxn",
	apiAddOffsetsToTxn:              "AddOffsetsToTxn",
	apiEndTxn:                       "EndTxn",
	apiWriteTxnMarkers:              "WriteTxnMarkers",
	apiTxnOffsetCommit:              "TxnOffsetCommit",
	apiDescribeAcls:                 "DescribeAcls",
	apiCreateAcls:                   "CreateAcls",
	apiDeleteAcls:                   "DeleteAcls",
	apiDescribeConfigs:              "DescribeConfigs",
	apiAlterConfigs:                 "AlterConfigs",
	apiAlterReplicaLogDirs:          "AlterReplicaLogDirs",
	apiDescribeLogDirs:              "DescribeLogDirs",
	apiSaslAuthenticate:             "SaslAuthenticate",
	apiCreatePartitions:             "CreatePartitions",
	apiCreateDelegationToken:        "CreateDelegationToken",
	apiRenewDelegationToken:         "RenewDelegationToken",
	apiExpireDelegationToken:        "ExpireDelegationToken",
	apiDescribeDelegationToken:      "DescribeDelegationToken",
	apiDeleteGroups:                 "DeleteGroups",
	apiElectLeaders:                 "ElectLeaders",
	apiIncrementalAlterConfigs:      "IncrementalAlterConfigs",
	apiAlterPartitionReassignments:  "AlterPartitionReassignments",
	apiListPartitionReassignments:   "ListPartitionReassignments",
	apiOffsetDelete:                 "OffsetDelete",
	apiDescribeClientQuotas:         "DescribeClientQuotas",
	apiAlterClientQuotas:            "AlterClientQuotas",
	apiDescribeUserScramCredentials: "DescribeUserScramCredentials",
	apiAlterUserScramCredentials:    "AlterUserScramCredentials",
	apiVote:                         "Vote",
	apiBeginQuorumEpoch:             "BeginQuorumEpoch",
	apiEndQuorumEpoch:               "EndQuorumEpoch",
	apiDescribeQuorum:               "DescribeQuorum",
	apiAlterPartition:               "AlterPartition",
	apiUpdateFeatures:               "UpdateFeatures",
	apiEnvelope:                     "Envelope",
	apiDescribeCluster:              "DescribeCluster",
	apiDescribeProducers:            "DescribeProducers",
	apiDescribeTransactions:         "DescribeTransactions",
	apiListTransactions:             "ListTransactions",
	apiAllocateProducerIds:          "AllocateProducerIds",
}

const (
	topicTypeString = 0
	topicTypeUUID   = 1
)

type topicRequest struct {
	apiVersion []int16
	skip       int
	topicType  uint8
}

// topicRequestMap kafka 普通请求跳过字节映射
// 目标是为了解析 topicArray 中第 0 个 topic 字段并 skip 其余字节
//
// apiVersion -1 表示所有版本
var topicRequestMap = map[apiKey][]topicRequest{
	apiFetch: {
		// apiVersion: 0-3
		// - replica_id(4)
		// - max_wait_ms(4)
		// - min_bytes(4)
		// - isolation_level(1)
		// - session_id(4)
		// - session_epoch(4)
		{apiVersion: []int16{0, 1, 2, 3, 4}, skip: 21},

		// apiVersion: 4-6
		// - replica_id(4)
		// - max_wait_ms(4)
		// - min_bytes(4)
		// - max_bytes(4)
		// - isolation_level(1)
		{apiVersion: []int16{4, 5, 6}, skip: 17},

		// apiVersion: 7-12
		// - replica_id(4)
		// - max_wait_ms(4)
		// - min_bytes(4)
		// - max_bytes(4)
		// - isolation_level(1)
		// - session_id(4)
		// - session_epoch(4)
		{apiVersion: []int16{7, 8, 9, 10, 11, 12, 13, 14}, skip: 25},

		// apiVersion: 15+
		// - max_wait_ms(4)
		// - min_bytes(4)
		// - max_bytes(4)
		// - isolation_level(1)
		// - session_id(4)
		// - session_epoch(4)
		{apiVersion: []int16{15, 16, 17}, skip: 21, topicType: topicTypeUUID},
	},

	apiListOffsets: {
		// apiVersion: 1
		// - replica_id(4)
		{apiVersion: []int16{1}, skip: 4},

		// apiVersion: 2-10
		// - replica_id(4)
		// - isolation_level(1)
		{apiVersion: []int16{2, 3, 4, 5, 6, 7, 8, 9, 10}, skip: 5},
	},

	apiMetadata: {
		// apiVersion: -1
		{apiVersion: []int16{-1}, skip: 0},
	},

	apiApiVersions: {
		// apiVersion: -1
		{apiVersion: []int16{-1}, skip: 0},
	},

	apiCreateTopics: {
		// apiVersion: -1
		{apiVersion: []int16{-1}, skip: 0},
	},

	apiDeleteTopics: {
		// apiVersion: -1
		{apiVersion: []int16{-1}, skip: 0},
	},

	apiCreatePartitions: {
		// apiVersion: -1
		{apiVersion: []int16{-1}, skip: 0},
	},

	apiAlterPartitionReassignments: {
		// apiVersion: -1
		{apiVersion: []int16{-1}, skip: 4},
	},

	apiListPartitionReassignments: {
		// apiVersion: -1
		{apiVersion: []int16{-1}, skip: 4},
	},

	apiDescribeProducers: {
		// apiVersion: -1
		{apiVersion: []int16{-1}, skip: 0},
	},
}

func matchTopicRequest(ak apiKey, version int16) (topicRequest, bool) {
	rs, ok := topicRequestMap[ak]
	if !ok {
		return topicRequest{}, false
	}

	if len(rs) == 1 && len(rs[0].apiVersion) == 1 && rs[0].apiVersion[0] == -1 {
		return rs[0], true
	}

	for i := 0; i < len(rs); i++ {
		for _, v := range rs[i].apiVersion {
			if v == version {
				return rs[i], true
			}
		}
	}
	return topicRequest{}, false
}

type op uint8

const (
	opInt16 op = iota
	opInt32
	opInt64
	opString
	opUvarint
	opGroupID
)

type fieldRequest struct {
	apiVersion []int16
	ops        []op
	withTopic  bool // 是否需要解析 Topic
	compact    bool // 是否使用紧凑数据格式
}

// fieldRequestMap kafka `字段`请求解析规则
// 可根据 op 要求逐个字段解析并 skip 其余字段
//
// apiVersion -1 表示所有版本
var fieldRequestMap = map[apiKey][]fieldRequest{
	apiProduce: {
		// apiVersion: -1
		// - transactional_id(string)
		// - acks(int16)
		// - timeout_ms(int32)
		{apiVersion: []int16{-1}, ops: []op{opString, opInt16, opInt32}, withTopic: true},
	},

	apiOffsetCommit: {
		// apiVersion: 2-4
		// - group_id(string)
		// - generation_id_or_member_epoch(int32)
		// - member_id(string)
		// - retention_time_ms(int64)
		{apiVersion: []int16{2, 3, 4}, ops: []op{opGroupID, opInt32, opString, opInt64}, withTopic: true},

		// apiVersion: 5-6
		// - group_id(string)
		// - generation_id_or_member_epoch(int32)
		// - member_id(string)
		{apiVersion: []int16{5, 6}, ops: []op{opGroupID, opInt32, opString}, withTopic: true},

		// apiVersion: 7-9
		// - group_id(string)
		// - generation_id_or_member_epoch(int32)
		// - member_id(string)
		// - group_instance_id(string)
		{apiVersion: []int16{7, 8, 9}, ops: []op{opGroupID, opInt32, opString, opString}, withTopic: true},
	},

	apiOffsetFetch: {
		// apiVersion: 1-7
		// - group_id(string)
		{apiVersion: []int16{1, 2, 3, 4, 5, 6, 7}, ops: []op{opGroupID}, withTopic: true},

		// apiVersion: 8-9
		// - groups(array)
		// - group_id(compact_string)
		//
		// 第一个字段 Int16 不是一个 `准确的` 值 因为在文档中没有明确说明其类型是否为紧凑类型
		// 据抓包观察 这个值大概率是两个字节长度的整数 因此定义为 Int16
		{apiVersion: []int16{8, 9}, ops: []op{opInt16, opGroupID, opString, opInt32}, withTopic: true, compact: true},
	},

	apiJoinGroup: {
		// apiVersion: -1
		// - group_id(string)
		{apiVersion: []int16{-1}, ops: []op{opGroupID}},
	},

	apiHeartbeat: {
		// apiVersion: -1
		// - group_id(string)
		{apiVersion: []int16{-1}, ops: []op{opGroupID}},
	},

	apiLeaveGroup: {
		// apiVersion: -1
		// - group_id(string)
		{apiVersion: []int16{-1}, ops: []op{opGroupID}},
	},

	apiSyncGroup: {
		// apiVersion: -1
		// - group_id(string)
		{apiVersion: []int16{-1}, ops: []op{opGroupID}},
	},

	apiDescribeGroups: {
		// apiVersion: -1
		// - groups(array)
		// - group_id(string)
		{apiVersion: []int16{-1}, ops: []op{opInt32, opGroupID}},
	},

	apiAddOffsetsToTxn: {
		// apiVersion: -1
		// - transactional_id(string)
		// - producer_id(int64)
		// - producer_epoch(int16)
		// - group_id(string)
		{apiVersion: []int16{-1}, ops: []op{opString, opInt64, opInt16, opGroupID}},
	},

	apiTxnOffsetCommit: {
		// apiVersion: 0-2
		// - transactional_id(string)
		// - group_id(string)
		// - producer_id(int64)
		// - producer_epoch(int16)
		{apiVersion: []int16{0, 1, 2}, ops: []op{opString, opGroupID, opInt64, opInt16}, withTopic: true},

		// apiVersion: 3-5
		// - transactional_id(string)
		// - group_id(compact_string)
		// - producer_id(int64)
		// - producer_epoch(int16)
		// - generation_id(int32)
		// - member_id(string)
		// - group_instance_id(string)
		{apiVersion: []int16{3, 4, 5}, ops: []op{opString, opGroupID, opInt64, opInt16, opInt32, opString, opString}, withTopic: true, compact: true},
	},

	apiDeleteGroups: {
		// apiVersion: -1
		// - groups(array)
		// - group_id(string)
		{apiVersion: []int16{-1}, ops: []op{opInt32, opGroupID}},
	},

	apiOffsetDelete: {
		// apiVersion: -1
		// - group_id(string)
		{apiVersion: []int16{-1}, ops: []op{opGroupID, opString, opInt32}, withTopic: true},
	},
}

func matchFieldRequest(ak apiKey, version int16) (fieldRequest, bool) {
	rs, ok := fieldRequestMap[ak]
	if !ok {
		return fieldRequest{}, false
	}

	if len(rs) == 1 && len(rs[0].apiVersion) == 1 && rs[0].apiVersion[0] == -1 {
		return rs[0], true
	}

	for i := 0; i < len(rs); i++ {
		for _, v := range rs[i].apiVersion {
			if v == version {
				return rs[i], true
			}
		}
	}
	return fieldRequest{}, false
}
