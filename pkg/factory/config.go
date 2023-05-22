package factory

import (
	"time"

	"github.com/davecgh/go-spew/spew"

	"github.com/free5gc/go-upf/internal/logger"
)

const (
	UpfDefaultConfigPath = "./config/upfcfg.yaml" // 默认配置文件地址
	UpfDefaultIPv4       = "127.0.0.8"            // UPF的默认地址
	UpfPfcpDefaultPort   = 8805                   // pfcp 服务（N4）默认端口
	UpfGtpDefaultPort    = 2152                   // N3 服务默认端口
)

type Config struct {
	Version     string    `yaml:"version"     valid:"required,in(1.0.3)"`
	Description string    `yaml:"description" valid:"optional"`
	Pfcp        *Pfcp     `yaml:"pfcp"        valid:"required"`
	GNB         *GNB      `yaml:"gnb" valid:"required"`
	UPFC        *UPFC     `yaml:"upfc" valid:"required"`
	Gtpu        *Gtpu     `yaml:"gtpu"        valid:"required"`
	DnnList     []DnnList `yaml:"dnnList"     valid:"required"`
	Logger      *Logger   `yaml:"logger"      valid:"required"`
}

type Pfcp struct {
	Addr           string        `yaml:"addr"           valid:"required,host"`
	NodeID         string        `yaml:"nodeID"         valid:"required,host"`
	RetransTimeout time.Duration `yaml:"retransTimeout" valid:"required"`
	MaxRetrans     uint8         `yaml:"maxRetrans"     valid:"optional"`
}

type UPFC struct {
	GRPC string `yaml:"grpc" valid:"required"`
}

type Gtpu struct {
	Forwarder string   `yaml:"forwarder" valid:"optional"`
	IfList    []IfInfo `yaml:"ifList"    valid:"optional"`
}

type GNB struct {
	Addr string `yaml:"addr"   valid:"required,host"`
	Mac  string `yaml:"mac"    valid:"required"`
}

type IfInfo struct {
	Addr    string `yaml:"addr"   valid:"required,host"`
	Type    string `yaml:"type"   valid:"required,in(N3|N6|N9)"`
	Mac     string `yaml:"Mac" valid:"optional"`
	PortNum uint32 `yaml:"portNumber"   valid:"optional"`
	Name    string `yaml:"name"   valid:"optional"`
	IfName  string `yaml:"ifname" valid:"optional"`
}

type DnnList struct {
	Dnn       string `yaml:"dnn"       valid:"required"`
	Cidr      string `yaml:"cidr"      valid:"required,cidr"`
	Dnnmac    string `yaml:"mac" valid:"optional"`
	Addr      string `yaml:"addr"   valid:"required,host"`
	NatIfName string `yaml:"natifname" valid:"optional"`
}

type Logger struct {
	Enable       bool   `yaml:"enable"       valid:"optional"`
	Level        string `yaml:"level"        valid:"required,in(trace|debug|info|warn|error|fatal|panic)"`
	ReportCaller bool   `yaml:"reportCaller" valid:"optional"`
}

func (c *Config) GetVersion() string {
	return c.Version
}

func (c *Config) Print() {
	spew.Config.Indent = "\t"
	str := spew.Sdump(c)
	logger.CfgLog.Infof("==================================================")
	logger.CfgLog.Infof("%s", str)
	logger.CfgLog.Infof("==================================================")
}
