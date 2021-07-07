package mcir

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/cloud-barista/cb-spider/interface/api"
	"github.com/cloud-barista/cb-tumblebug/src/core/common"
	"github.com/go-resty/resty/v2"
)

// 2020-04-13 https://github.com/cloud-barista/cb-spider/blob/master/cloud-control-manager/cloud-driver/interfaces/resources/SecurityHandler.go

type SpiderSecurityReqInfoWrapper struct { // Spider
	ConnectionName string
	ReqInfo        SpiderSecurityInfo
}

/*
type SpiderSecurityReqInfo struct { // Spider
	Name          string
	VPCName       string
	SecurityRules *[]SpiderSecurityRuleInfo
	//Direction     string // @todo used??
}
*/

type SpiderSecurityRuleInfo struct { // Spider
	FromPort   string //`json:"fromPort"`
	ToPort     string //`json:"toPort"`
	IPProtocol string //`json:"ipProtocol"`
	Direction  string //`json:"direction"`
	CIDR       string
}

type SpiderSecurityInfo struct { // Spider
	// Fields for request
	Name    string
	VPCName string

	// Fields for both request and response
	SecurityRules *[]SpiderSecurityRuleInfo

	// Fields for response
	IId          common.IID // {NameId, SystemId}
	VpcIID       common.IID // {NameId, SystemId}
	Direction    string     // @todo userd??
	KeyValueList []common.KeyValue
}

type TbSecurityGroupReq struct { // Tumblebug
	Name           string                    `json:"name"`
	ConnectionName string                    `json:"connectionName"`
	VNetId         string                    `json:"vNetId"`
	Description    string                    `json:"description"`
	FirewallRules  *[]SpiderSecurityRuleInfo `json:"firewallRules"`
}

type TbSecurityGroupInfo struct { // Tumblebug
	Id                   string                    `json:"id"`
	Name                 string                    `json:"name"`
	ConnectionName       string                    `json:"connectionName"`
	VNetId               string                    `json:"vNetId"`
	Description          string                    `json:"description"`
	FirewallRules        *[]SpiderSecurityRuleInfo `json:"firewallRules"`
	CspSecurityGroupId   string                    `json:"cspSecurityGroupId"`
	CspSecurityGroupName string                    `json:"cspSecurityGroupName"`
	KeyValueList         []common.KeyValue         `json:"keyValueList"`
	AssociatedObjectList []string                  `json:"associatedObjectList"`
	IsAutoGenerated      bool                      `json:"isAutoGenerated"`

	// Disabled for now
	//ResourceGroupName  string `json:"resourceGroupName"`
}

// CreateSecurityGroup accepts SG creation request, creates and returns an TB SG object
func CreateSecurityGroup(nsId string, u *TbSecurityGroupReq) (TbSecurityGroupInfo, error) {

	resourceType := common.StrSecurityGroup

	err := common.CheckString(nsId)
	if err != nil {
		temp := TbSecurityGroupInfo{}
		common.CBLog.Error(err)
		return temp, err
	}
	err = common.CheckString(u.Name)
	if err != nil {
		temp := TbSecurityGroupInfo{}
		common.CBLog.Error(err)
		return temp, err
	}
	check, err := CheckResource(nsId, resourceType, u.Name)

	if check {
		temp := TbSecurityGroupInfo{}
		err := fmt.Errorf("The securityGroup " + u.Name + " already exists.")
		//return temp, http.StatusConflict, nil, err
		return temp, err
	}
	if err != nil {
		common.CBLog.Error(err)
		content := TbSecurityGroupInfo{}
		err := fmt.Errorf("Cannot create securityGroup")
		return content, err
	}

	tempReq := SpiderSecurityReqInfoWrapper{}
	tempReq.ConnectionName = u.ConnectionName
	tempReq.ReqInfo.Name = u.Name
	tempReq.ReqInfo.VPCName = u.VNetId
	tempReq.ReqInfo.SecurityRules = u.FirewallRules

	var tempSpiderSecurityInfo *SpiderSecurityInfo

	if os.Getenv("SPIDER_CALL_METHOD") == "REST" {

		url := common.SPIDER_REST_URL + "/securitygroup"

		client := resty.New().SetCloseConnection(true)

		resp, err := client.R().
			SetHeader("Content-Type", "application/json").
			SetBody(tempReq).
			SetResult(&SpiderSecurityInfo{}). // or SetResult(AuthSuccess{}).
			//SetError(&AuthError{}).       // or SetError(AuthError{}).
			Post(url)

		if err != nil {
			common.CBLog.Error(err)
			content := TbSecurityGroupInfo{}
			err := fmt.Errorf("an error occurred while requesting to CB-Spider")
			return content, err
		}

		fmt.Println("HTTP Status code: " + strconv.Itoa(resp.StatusCode()))
		switch {
		case resp.StatusCode() >= 400 || resp.StatusCode() < 200:
			err := fmt.Errorf(string(resp.Body()))
			common.CBLog.Error(err)
			content := TbSecurityGroupInfo{}
			return content, err
		}

		tempSpiderSecurityInfo = resp.Result().(*SpiderSecurityInfo)

	} else {

		// CCM API 설정
		ccm := api.NewCloudResourceHandler()
		err := ccm.SetConfigPath(os.Getenv("CBTUMBLEBUG_ROOT") + "/conf/grpc_conf.yaml")
		if err != nil {
			common.CBLog.Error("ccm failed to set config : ", err)
			return TbSecurityGroupInfo{}, err
		}
		err = ccm.Open()
		if err != nil {
			common.CBLog.Error("ccm api open failed : ", err)
			return TbSecurityGroupInfo{}, err
		}
		defer ccm.Close()

		payload, _ := json.Marshal(tempReq)
		fmt.Println("payload: " + string(payload)) // for debug

		result, err := ccm.CreateSecurity(string(payload))
		if err != nil {
			common.CBLog.Error(err)
			return TbSecurityGroupInfo{}, err
		}

		tempSpiderSecurityInfo = &SpiderSecurityInfo{}
		err = json.Unmarshal([]byte(result), &tempSpiderSecurityInfo)
		if err != nil {
			common.CBLog.Error(err)
			return TbSecurityGroupInfo{}, err
		}
	}

	content := TbSecurityGroupInfo{}
	//content.Id = common.GenUuid()
	content.Id = u.Name
	content.Name = u.Name
	content.ConnectionName = u.ConnectionName
	content.VNetId = tempSpiderSecurityInfo.VpcIID.NameId
	content.CspSecurityGroupId = tempSpiderSecurityInfo.IId.SystemId
	content.CspSecurityGroupName = tempSpiderSecurityInfo.IId.NameId
	content.Description = u.Description
	content.FirewallRules = tempSpiderSecurityInfo.SecurityRules
	content.KeyValueList = tempSpiderSecurityInfo.KeyValueList
	content.AssociatedObjectList = []string{}

	// cb-store
	fmt.Println("=========================== PUT CreateSecurityGroup")
	Key := common.GenResourceKey(nsId, resourceType, content.Id)
	Val, _ := json.Marshal(content)
	err = common.CBStore.Put(string(Key), string(Val))
	if err != nil {
		common.CBLog.Error(err)
		return content, err
	}
	keyValue, _ := common.CBStore.Get(string(Key))
	fmt.Println("<" + keyValue.Key + "> \n" + keyValue.Value)
	fmt.Println("===========================")
	return content, nil
}
