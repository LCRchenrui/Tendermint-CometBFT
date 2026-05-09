package model

// 定义共识交易类型和交易结构

type CommandType string

// 定义了三种交易类型
const (
	CommandDeploy   CommandType = "deploy"
	CommandInstance CommandType = "instance"
	CommandComplete CommandType = "complete"
)

type Tx struct {
	TxID    string      `json:"txId"`
	Type    CommandType `json:"type"`
	OID     string      `json:"oid,omitempty"`
	Payload Payload     `json:"payload"`
}

type Payload struct {
	DeploymentName    string `json:"deploymentName,omitempty"`
	FileContent       string `json:"fileContent,omitempty"`
	Signatures        string `json:"signatures,omitempty"`
	BusinessData      string `json:"businessData,omitempty"`
	ProcessData       string `json:"processData,omitempty"`
	TaskName          string `json:"taskName,omitempty"`
	User              string `json:"user,omitempty"`
	ServiceTaskResult string `json:"serviceTaskResultJson,omitempty"`
	StaticAllocation  string `json:"staticAllocationTable,omitempty"`
}

type CommandRecord struct {
	Tx              Tx     `json:"tx"`
	ConsensusStatus string `json:"consensusStatus"`
	ExecutionStatus string `json:"executionStatus"`
	ResultHash      string `json:"resultHash,omitempty"`
	ResultBody      string `json:"resultBody,omitempty"`
	ErrorMessage    string `json:"errorMessage,omitempty"`
}

type AppState struct {
	Height   int64                    `json:"height"`
	Commands map[string]CommandRecord `json:"commands"`
	Queue    []string                 `json:"queue"`
}
