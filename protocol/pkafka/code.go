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

// errorCode 标识着 Kafka 协议 `顶层` broker 的错误码
//
// 详见 https://kafka.apache.org/protocol#protocol_error_codes
type errorCode int16

const (
	codeNoError                            errorCode = 0   // 成功
	codeUnknownServerError                 errorCode = -1  // 未知服务端错误（非协议定义值）
	codeOffsetOutOfRange                   errorCode = 1   // 请求偏移量越界
	codeCorruptMessage                     errorCode = 2   // 消息校验失败
	codeUnknownTopicOrPartition            errorCode = 3   // 主题/分区不存在
	codeInvalidFetchSize                   errorCode = 4   // 请求大小非法
	codeLeaderNotAvailable                 errorCode = 5   // 分区 Leader 不可用
	codeNotLeaderOrFollower                errorCode = 6   // 当前节点非 Leader/Follower
	codeRequestTimedOut                    errorCode = 7   // 请求超时
	codeBrokerNotAvailable                 errorCode = 8   // Broker 不可用
	codeReplicaNotAvailable                errorCode = 9   // 副本不可用
	codeMessageSizeTooLarge                errorCode = 10  // 消息尺寸过大
	codeStaleControllerEpoch               errorCode = 11  // Controller 版本过期
	codeOffsetMetadataTooLarge             errorCode = 12  // 偏移元数据过大
	codeNetworkException                   errorCode = 13  // 网络中断
	codeCoordinatorLoadInProgress          errorCode = 14  // 协调器正在加载
	codeCoordinatorNotAvailable            errorCode = 15  // 协调器不可用
	codeNotCoordinator                     errorCode = 16  // 非协调器节点
	codeInvalidTopicException              errorCode = 17  // 非法主题名称
	codeRecordListTooLarge                 errorCode = 18  // 批量消息过大
	codeNotEnoughReplicas                  errorCode = 19  // 副本数不足
	codeNotEnoughReplicasAfterAppend       errorCode = 20  // 写入后副本不足
	codeInvalidRequiredAcks                errorCode = 21  // 非法的 ACK 配置
	codeIllegalGeneration                  errorCode = 22  // 消费组 Generation ID 非法
	codeInconsistentGroupProtocol          errorCode = 23  // 协议类型不一致
	codeInvalidGroupId                     errorCode = 24  // 消费组 ID 非法
	codeUnknownMemberId                    errorCode = 25  // 消费者成员 ID 未知
	codeInvalidSessionTimeout              errorCode = 26  // 会话超时时间非法
	codeRebalanceInProgress                errorCode = 27  // 消费组正在重平衡
	codeInvalidCommitOffsetSize            errorCode = 28  // 提交偏移量非法
	codeTopicAuthorizationFailed           errorCode = 29  // 主题操作未授权
	codeGroupAuthorizationFailed           errorCode = 30  // 消费组操作未授权
	codeClusterAuthorizationFailed         errorCode = 31  // 集群操作未授权
	codeInvalidTimestamp                   errorCode = 32  // 消息时间戳非法
	codeUnsupportedSASLMechanism           errorCode = 33  // SASL 机制不支持
	codeIllegalSASLState                   errorCode = 34  // SASL 状态非法
	codeUnsupportedVersion                 errorCode = 35  // 协议版本不支持
	codeTopicAlreadyExists                 errorCode = 36  // 主题已存在
	codeInvalidPartitions                  errorCode = 37  // 分区数非法
	codeInvalidReplicationFactor           errorCode = 38  // 副本因子非法
	codeInvalidReplicaAssignment           errorCode = 39  // 副本分配方案非法
	codeInvalidConfig                      errorCode = 40  // 配置参数非法
	codeNotController                      errorCode = 41  // 非 Controller 节点
	codeInvalidRequest                     errorCode = 42  // 请求格式非法
	codeUnsupportedForMessageFormat        errorCode = 43  // 消息格式不支持
	codePolicyViolation                    errorCode = 44  // 违反策略限制
	codeOutOfOrderSequenceNumber           errorCode = 45  // 序列号乱序
	codeDuplicateSequenceNumber            errorCode = 46  // 序列号重复
	codeInvalidProducerEpoch               errorCode = 47  // 生产者 Epoch 失效
	codeInvalidTxnState                    errorCode = 48  // 事务状态非法
	codeInvalidProducerIDMapping           errorCode = 49  // Producer ID 映射失效
	codeInvalidTransactionTimeout          errorCode = 50  // 事务超时设置非法
	codeConcurrentTransactions             errorCode = 51  // 事务并发冲突
	codeTransactionCoordinatorFenced       errorCode = 52  // 事务协调器隔离
	codeTransactionalIDAuthorizationFailed errorCode = 53  // 事务ID未授权
	codeSecurityDisabled                   errorCode = 54  // 安全功能未启用
	codeOperationNotAttempted              errorCode = 55  // 操作未执行
	codeKafkaStorageError                  errorCode = 56  // 存储系统错误
	codeLogDirNotFound                     errorCode = 57  // 日志目录未找到
	codeSASLAuthenticationFailed           errorCode = 58  // SASL 认证失败
	codeUnknownProducerID                  errorCode = 59  // Producer ID 未知
	codeReassignmentInProgress             errorCode = 60  // 分区正在迁移
	codeDelegationTokenAuthDisabled        errorCode = 61  // Token 认证未启用
	codeDelegationTokenNotFound            errorCode = 62  // Token 不存在
	codeDelegationTokenOwnerMismatch       errorCode = 63  // Token 所属不匹配
	codeDelegationTokenRequestNotAllowed   errorCode = 64  // Token 请求非法
	codeDelegationTokenAuthorizationFailed errorCode = 65  // Token 授权失败
	codeDelegationTokenExpired             errorCode = 66  // Token 已过期
	codeInvalidPrincipalType               errorCode = 67  // 主体类型非法
	codeNonEmptyGroup                      errorCode = 68  // 消费组非空
	codeGroupIDNotFound                    errorCode = 69  // 消费组 ID 未找到
	codeFetchSessionIDNotFound             errorCode = 70  // Fetch 会话 ID 不存在
	codeInvalidFetchSessionEpoch           errorCode = 71  // Fetch 会话 Epoch 非法
	codeListenerNotFound                   errorCode = 72  // 监听器未找到
	codeTopicDeletionDisabled              errorCode = 73  // 主题删除被禁用
	codeFencedLeaderEpoch                  errorCode = 74  // Leader Epoch 过期
	codeUnknownLeaderEpoch                 errorCode = 75  // Leader Epoch 未知
	codeUnsupportedCompressionType         errorCode = 76  // 压缩类型不支持
	codeStaleBrokerEpoch                   errorCode = 77  // Broker Epoch 过期
	codeOffsetNotAvailable                 errorCode = 78  // 偏移量不可用
	codeMemberIdRequired                   errorCode = 79  // 需要成员 ID
	codePreferredLeaderNotAvailable        errorCode = 80  // 首选 Leader 不可用
	codeGroupMaxSizeReached                errorCode = 81  // 消费组人数达上限
	codeFencedInstanceId                   errorCode = 82  // 实例 ID 被隔离
	codeEligibleLeadersNotAvailable        errorCode = 83  // 无合格 Leader
	codeElectionNotNeeded                  errorCode = 84  // 无需选举
	codeNoReassignmentInProgress           errorCode = 85  // 无迁移任务进行
	codeGroupSubscribedToTopic             errorCode = 86  // 消费组已订阅主题
	codeInvalidRecord                      errorCode = 87  // 记录非法
	codeUnstableOffsetCommit               errorCode = 88  // 偏移提交不稳定
	codeThrottlingQuotaExceeded            errorCode = 89  // 限流配额超限
	codeProducerFenced                     errorCode = 90  // 生产者被隔离
	codeResourceNotFound                   errorCode = 91  // 资源未找到
	codeDuplicateResource                  errorCode = 92  // 资源重复
	codeUnacceptableCredential             errorCode = 93  // 凭证不合法
	codeInconsistentVoterSet               errorCode = 94  // 投票集合不一致
	codeInvalidUpdateVersion               errorCode = 95  // 更新版本非法
	codeFeatureUpdateFailed                errorCode = 96  // 功能更新失败
	codePrincipalDeserializationFailure    errorCode = 97  // 主体反序列化失败
	codeSnapshotNotFound                   errorCode = 98  // 快照未找到
	codePositionOutOfRange                 errorCode = 99  // 位移越界
	codeUnknownTopicID                     errorCode = 100 // 主题 ID 未知
	codeDuplicateBrokerRegistration        errorCode = 101 // Broker 注册重复
	codeBrokerIDNotRegistered              errorCode = 102 // Broker ID 未注册
	codeInconsistentTopicID                errorCode = 103 // 主题 ID 不一致
	codeInconsistentClusterID              errorCode = 104 // 集群 ID 不一致
	codeTransactionalIDNotFound            errorCode = 105 // 事务 ID 未找到
	codeFetchSessionTopicIDError           errorCode = 106 // Fetch 会话主题 ID 错误
)

var errCodes = map[errorCode]string{
	codeNoError:                            "NoError",
	codeUnknownServerError:                 "UnknownServerError",
	codeOffsetOutOfRange:                   "OffsetOutOfRange",
	codeCorruptMessage:                     "CorruptMessage",
	codeUnknownTopicOrPartition:            "UnknownTopicOrPartition",
	codeInvalidFetchSize:                   "InvalidFetchSize",
	codeLeaderNotAvailable:                 "LeaderNotAvailable",
	codeNotLeaderOrFollower:                "NotLeaderOrFollower",
	codeRequestTimedOut:                    "RequestTimedOut",
	codeBrokerNotAvailable:                 "BrokerNotAvailable",
	codeReplicaNotAvailable:                "ReplicaNotAvailable",
	codeMessageSizeTooLarge:                "MessageSizeTooLarge",
	codeStaleControllerEpoch:               "StaleControllerEpoch",
	codeOffsetMetadataTooLarge:             "OffsetMetadataTooLarge",
	codeNetworkException:                   "NetworkException",
	codeCoordinatorLoadInProgress:          "CoordinatorLoadInProgress",
	codeCoordinatorNotAvailable:            "CoordinatorNotAvailable",
	codeNotCoordinator:                     "NotCoordinator",
	codeInvalidTopicException:              "InvalidTopicException",
	codeRecordListTooLarge:                 "RecordListTooLarge",
	codeNotEnoughReplicas:                  "NotEnoughReplicas",
	codeNotEnoughReplicasAfterAppend:       "NotEnoughReplicasAfterAppend",
	codeInvalidRequiredAcks:                "InvalidRequiredAcks",
	codeIllegalGeneration:                  "IllegalGeneration",
	codeInconsistentGroupProtocol:          "InconsistentGroupProtocol",
	codeInvalidGroupId:                     "InvalidGroupId",
	codeUnknownMemberId:                    "UnknownMemberId",
	codeInvalidSessionTimeout:              "InvalidSessionTimeout",
	codeRebalanceInProgress:                "RebalanceInProgress",
	codeInvalidCommitOffsetSize:            "InvalidCommitOffsetSize",
	codeTopicAuthorizationFailed:           "TopicAuthorizationFailed",
	codeGroupAuthorizationFailed:           "GroupAuthorizationFailed",
	codeClusterAuthorizationFailed:         "ClusterAuthorizationFailed",
	codeInvalidTimestamp:                   "InvalidTimestamp",
	codeUnsupportedSASLMechanism:           "UnsupportedSASLMechanism",
	codeIllegalSASLState:                   "IllegalSASLState",
	codeUnsupportedVersion:                 "UnsupportedVersion",
	codeTopicAlreadyExists:                 "TopicAlreadyExists",
	codeInvalidPartitions:                  "InvalidPartitions",
	codeInvalidReplicationFactor:           "InvalidReplicationFactor",
	codeInvalidReplicaAssignment:           "InvalidReplicaAssignment",
	codeInvalidConfig:                      "InvalidConfig",
	codeNotController:                      "NotController",
	codeInvalidRequest:                     "InvalidRequest",
	codeUnsupportedForMessageFormat:        "UnsupportedForMessageFormat",
	codePolicyViolation:                    "PolicyViolation",
	codeOutOfOrderSequenceNumber:           "OutOfOrderSequenceNumber",
	codeDuplicateSequenceNumber:            "DuplicateSequenceNumber",
	codeInvalidProducerEpoch:               "InvalidProducerEpoch",
	codeInvalidTxnState:                    "InvalidTxnState",
	codeInvalidProducerIDMapping:           "InvalidProducerIDMapping",
	codeInvalidTransactionTimeout:          "InvalidTransactionTimeout",
	codeConcurrentTransactions:             "ConcurrentTransactions",
	codeTransactionCoordinatorFenced:       "TransactionCoordinatorFenced",
	codeTransactionalIDAuthorizationFailed: "TransactionalIDAuthorizationFailed",
	codeSecurityDisabled:                   "SecurityDisabled",
	codeOperationNotAttempted:              "OperationNotAttempted",
	codeKafkaStorageError:                  "KafkaStorageError",
	codeLogDirNotFound:                     "LogDirNotFound",
	codeSASLAuthenticationFailed:           "SASLAuthenticationFailed",
	codeUnknownProducerID:                  "UnknownProducerID",
	codeReassignmentInProgress:             "ReassignmentInProgress",
	codeDelegationTokenAuthDisabled:        "DelegationTokenAuthDisabled",
	codeDelegationTokenNotFound:            "DelegationTokenNotFound",
	codeDelegationTokenOwnerMismatch:       "DelegationTokenOwnerMismatch",
	codeDelegationTokenRequestNotAllowed:   "DelegationTokenRequestNotAllowed",
	codeDelegationTokenAuthorizationFailed: "DelegationTokenAuthorizationFailed",
	codeDelegationTokenExpired:             "DelegationTokenExpired",
	codeInvalidPrincipalType:               "InvalidPrincipalType",
	codeNonEmptyGroup:                      "NonEmptyGroup",
	codeGroupIDNotFound:                    "GroupIDNotFound",
	codeFetchSessionIDNotFound:             "FetchSessionIDNotFound",
	codeInvalidFetchSessionEpoch:           "InvalidFetchSessionEpoch",
	codeListenerNotFound:                   "ListenerNotFound",
	codeTopicDeletionDisabled:              "TopicDeletionDisabled",
	codeFencedLeaderEpoch:                  "FencedLeaderEpoch",
	codeUnknownLeaderEpoch:                 "UnknownLeaderEpoch",
	codeUnsupportedCompressionType:         "UnsupportedCompressionType",
	codeStaleBrokerEpoch:                   "StaleBrokerEpoch",
	codeOffsetNotAvailable:                 "OffsetNotAvailable",
	codeMemberIdRequired:                   "MemberIdRequired",
	codePreferredLeaderNotAvailable:        "PreferredLeaderNotAvailable",
	codeGroupMaxSizeReached:                "GroupMaxSizeReached",
	codeFencedInstanceId:                   "FencedInstanceId",
	codeEligibleLeadersNotAvailable:        "EligibleLeadersNotAvailable",
	codeElectionNotNeeded:                  "ElectionNotNeeded",
	codeNoReassignmentInProgress:           "NoReassignmentInProgress",
	codeGroupSubscribedToTopic:             "GroupSubscribedToTopic",
	codeInvalidRecord:                      "InvalidRecord",
	codeUnstableOffsetCommit:               "UnstableOffsetCommit",
	codeThrottlingQuotaExceeded:            "ThrottlingQuotaExceeded",
	codeProducerFenced:                     "ProducerFenced",
	codeResourceNotFound:                   "ResourceNotFound",
	codeDuplicateResource:                  "DuplicateResource",
	codeUnacceptableCredential:             "UnacceptableCredential",
	codeInconsistentVoterSet:               "InconsistentVoterSet",
	codeInvalidUpdateVersion:               "InvalidUpdateVersion",
	codeFeatureUpdateFailed:                "FeatureUpdateFailed",
	codePrincipalDeserializationFailure:    "PrincipalDeserializationFailure",
	codeSnapshotNotFound:                   "SnapshotNotFound",
	codePositionOutOfRange:                 "PositionOutOfRange",
	codeUnknownTopicID:                     "UnknownTopicID",
	codeDuplicateBrokerRegistration:        "DuplicateBrokerRegistration",
	codeBrokerIDNotRegistered:              "BrokerIDNotRegistered",
	codeInconsistentTopicID:                "InconsistentTopicID",
	codeInconsistentClusterID:              "InconsistentClusterID",
	codeTransactionalIDNotFound:            "TransactionalIDNotFound",
	codeFetchSessionTopicIDError:           "FetchSessionTopicIDError",
}
