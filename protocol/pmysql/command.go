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

package pmysql

const (
	cmdQuit            = 0x01 // COM_QUIT 关闭连接请求
	cmdInitDB          = 0x02 // COM_INIT_DB 切换数据库（等价于 USE database）
	cmdQuery           = 0x03 // COM_QUERY 执行 SQL 查询（如 SELECT / INSERT 等）
	cmdFieldList       = 0x04 // COM_FIELD_LIST 列出表字段（类似 SHOW COLUMNS）
	cmdCreateDB        = 0x05 // COM_CREATE_DB 创建数据库（已废弃 改用 COM_QUERY）
	cmdDropDB          = 0x06 // COM_DROP_DB 删除数据库（已废弃 改用 COM_QUERY）
	cmdRefresh         = 0x07 // COM_REFRESH 刷新缓存（如 FLUSH TABLES）
	cmdShutdown        = 0x08 // COM_SHUTDOWN 关闭服务器
	cmdStatistics      = 0x09 // COM_STATISTICS 获取服务器统计信息
	cmdProcess         = 0x0A // COM_PROCESS_INFO 获取进程列表（等价于 SHOW PROCESSLIST）
	cmdConnect         = 0x0B // COM_CONNECT 内部保留（未使用）
	cmdKill            = 0x0C // COM_KILL 终止指定线程（等价于 KILL <thread_id>)
	cmdDebug           = 0x0D // COM_DEBUG 转储调试信息到日志
	cmdPing            = 0x0E // COM_PING 检查服务器存活（心跳包）
	cmdTime            = 0x0F // COM_TIME 内部保留（未使用）
	cmdDelayed         = 0x10 // COM_DELAYED_INSERT 内部保留（未使用）
	cmdChangeUser      = 0x11 // COM_CHANGE_USER 切换用户（重新认证）
	cmdBinlogDump      = 0x12 // COM_BINLOG_DUMP 请求二进制日志（主从复制）
	cmdTableDump       = 0x13 // COM_TABLE_DUMP 转储表数据（内部使用）
	cmdConnectOut      = 0x14 // COM_CONNECT_OUT 内部保留（未使用）
	cmdRegister        = 0x15 // COM_REGISTER_SLAVE 从服务器向主服务器注册（复制场景）
	cmdPrepare         = 0x16 // COM_STMT_PREPARE 预处理语句 准备 SQL 语句（如 PREPARE stmt FROM '...')
	cmdExecute         = 0x17 // COM_STMT_EXECUTE 预处理语句 执行已准备的语句
	cmdSendLongData    = 0x18 // COM_STMT_SEND_LONG_DATA 预处理语句：发送长数据（如 BLOB）
	cmdCloseStmt       = 0x19 // COM_STMT_CLOSE 预处理语句 关闭已准备的语句
	cmdResetStmt       = 0x1A // COM_STMT_RESET 预处理语句 重置参数和结果集
	cmdSetOption       = 0x1B // COM_SET_OPTION 设置连接选项（如 SET NAMES utf8）
	cmdFetchStmt       = 0x1C // COM_STMT_FETCH 预处理语句 从结果集中获取更多行
	cmdDaemon          = 0x1D // COM_DAEMON 内部保留（未使用）
	cmdBinlogDumpGTID  = 0x1E // COM_BINLOG_DUMP_GTID 基于 GTID 的二进制日志请求（MySQL 5.6+）
	cmdResetConnection = 0x1F // COM_RESET_CONNECTION 重置连接状态（清理会话变量 MySQL 8.0+）
)

var commands = map[uint8]string{
	cmdQuit:            "QUIT",
	cmdInitDB:          "INIT_DB",
	cmdQuery:           "QUERY",
	cmdFieldList:       "FIELD_LIST",
	cmdCreateDB:        "CREATE_DB",
	cmdDropDB:          "DROP_DB",
	cmdRefresh:         "REFRESH",
	cmdShutdown:        "SHUTDOWN",
	cmdStatistics:      "STATISTICS",
	cmdProcess:         "PROCESS",
	cmdConnect:         "CONNECT",
	cmdKill:            "KILL",
	cmdDebug:           "DEBUG",
	cmdPing:            "PING",
	cmdTime:            "TIME",
	cmdDelayed:         "DELAYED",
	cmdChangeUser:      "CHANGE_USER",
	cmdBinlogDump:      "BINLOG_DUMP",
	cmdTableDump:       "TABLE_DUMP",
	cmdConnectOut:      "CONNECT_OUT",
	cmdRegister:        "REGISTER",
	cmdPrepare:         "PREPARE",
	cmdExecute:         "EXECUTE",
	cmdSendLongData:    "SEND_LONG_DATA",
	cmdCloseStmt:       "CLOSE_STMT",
	cmdResetStmt:       "RESET_STMT",
	cmdSetOption:       "SET_OPTION",
	cmdFetchStmt:       "FETCH_STMT",
	cmdDaemon:          "DAEMON",
	cmdBinlogDumpGTID:  "BINLOG_DUMP_GTID",
	cmdResetConnection: "RESET_CONNECTION",
}

const (
	packetOK          = 0x00 // 执行成功响应 包含 AffectedRows 和 LastInsetID
	packetError       = 0xFF // 执行失败响应 包含错误码和错误消息
	packetEOF         = 0xFE // 标识结果集列定义结束或结果集数据传输结束（旧协议中可能用 0xFE 替代 OK 包）
	packetAuthSwitch  = 0x01 // 认证切换请求（服务端要求客户端更换认证方式）
	packetLocalInfile = 0xFB // 服务端请求客户端发送本地文件（用于 LOAD DATA LOCAL INFILE 命令）
)
