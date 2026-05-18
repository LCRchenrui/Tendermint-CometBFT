package model

// 定义共识交易类型和交易结构

type CommandType string

// 定义了三种交易类型
const (
	CommandDeploy      CommandType = "deploy"
	CommandInstance    CommandType = "instance"
	CommandComplete    CommandType = "complete"
	CommandNacosPut    CommandType = "nacos_put"
	CommandNacosDelete CommandType = "nacos_delete"
)

type Tx struct {
	TxID      string             `json:"txId"`
	Type      CommandType        `json:"type"`
	OID       string             `json:"oid,omitempty"`
	Payload   Payload            `json:"payload"`
	Execution *ExecutionEnvelope `json:"execution,omitempty"`
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
	Key               string `json:"key,omitempty"`
	Value             string `json:"value,omitempty"`
}

type ExecutionEnvelope struct {
	PreparedAtHeight int64             `json:"preparedAtHeight,omitempty"`
	PreparedBy       string            `json:"preparedBy,omitempty"`
	ServiceProof     string            `json:"serviceProof,omitempty"`
	Result           WorkflowConsensus `json:"result"`
}

type WorkflowConsensus struct {
	OID               string   `json:"oid,omitempty"`
	FromTaskNames     []string `json:"fromTaskNames,omitempty"`
	ToTaskNames       []string `json:"toTaskNames,omitempty"`
	IsDeploy          bool     `json:"isDeploy,omitempty"`
	DeploymentName    string   `json:"deploymentName,omitempty"`
	BusinessData      string   `json:"businessData,omitempty"`
	ServiceTaskResult string   `json:"serviceTaskResultJson,omitempty"`
	IsEnd             bool     `json:"isEnd,omitempty"`
	ReadSet           string   `json:"readSetJson,omitempty"`
	WriteSet          string   `json:"writeSetJson,omitempty"`
	ServiceURLs       []string `json:"serviceUrls,omitempty"`
}

type CommandRecord struct {
	Tx                  Tx                `json:"tx"`
	ConsensusStatus     string            `json:"consensusStatus"`
	ExecutionStatus     string            `json:"executionStatus"`
	ResultHash          string            `json:"resultHash,omitempty"`
	ResultBody          string            `json:"resultBody,omitempty"`
	PreparedResultHash  string            `json:"preparedResultHash,omitempty"`
	FinalizedResultHash string            `json:"finalizedResultHash,omitempty"`
	Result              WorkflowConsensus `json:"result,omitempty"`
	ErrorMessage        string            `json:"errorMessage,omitempty"`
}

type AppState struct {
	Height   int64                    `json:"height"`
	Commands map[string]CommandRecord `json:"commands"`
	NacosKV  map[string]string        `json:"nacosKv,omitempty"`
	AppHash  string                   `json:"appHash,omitempty"`
}
