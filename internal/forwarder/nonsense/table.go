package forwarder

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"github.com/free5gc/go-upf/pkg/factory"
	"net"
	"strconv"
	"strings"

	"github.com/free5gc/go-upf/internal/forwarder/utils"

	p4config "github.com/p4lang/p4runtime/go/p4/config/v1"
	p4 "github.com/p4lang/p4runtime/go/p4/v1"
)

/*
针对表的操作都放在这里, 我的想法是一个表一个class，这样就不用考虑分情况了
1. 根据表项的条件来确定是何种匹配类型
2. 根据匹配类型决定match key的解析方式
3.
*/

type Operator struct {
	table   *p4config.Table
	tableID uint32
	p4info  *p4config.P4Info
	cfg     *factory.Config
}

func NewOperator(tableName string, p4info *p4config.P4Info) *Operator {
	operator := Operator{
		p4info: p4info,
	}
	for _, item := range p4info.GetTables() {
		if item.GetPreamble().Name == tableName {
			operator.table = item
			operator.tableID = item.GetPreamble().Id
		}
	}

	return &operator
}

func (opt *Operator) EntryBuilder4(matchKeys []*MatchKeys, actionProfileGroupID int, priority int32) (*p4.TableEntry, error) {

	matchFields, _ := opt.matchFieldBuilder(matchKeys)
	entryAction := p4.TableAction{
		Type: &p4.TableAction_ActionProfileGroupId{
			ActionProfileGroupId: uint32(actionProfileGroupID),
		},
	}
	request := p4.TableEntry{
		Match:    matchFields,
		Action:   &entryAction,
		TableId:  opt.tableID,
		Priority: priority,
	}
	return &request, nil
}

func (opt *Operator) EntryBuilder3(matchKeysStringSlice []string, actionProfileGroupID int, priority int32) (*p4.TableEntry, error) {

	matchkeys, err := ParseMatchKeysString(matchKeysStringSlice)
	if err != nil {
		return nil, err
	}
	matchFields, _ := opt.matchFieldBuilder(matchkeys)
	entryAction := p4.TableAction{
		Type: &p4.TableAction_ActionProfileGroupId{
			ActionProfileGroupId: uint32(actionProfileGroupID),
		},
	}
	request := p4.TableEntry{
		Match:    matchFields,
		Action:   &entryAction,
		TableId:  opt.tableID,
		Priority: priority,
	}
	return &request, nil
}

func (opt *Operator) EntryBuilder2(matchKeysStringSlice []string, actionName string, actionParamsStringSlice []string, priority int32) (*p4.TableEntry, error) {

	matchkeys, err := ParseMatchKeysString(matchKeysStringSlice)
	if err != nil {
		return nil, err
	}
	actionParams, err := HexStringSliceToByteSlice(actionParamsStringSlice)

	if err != nil {
		return nil, err
	}
	return opt.EntryBuilder(matchkeys, actionName, actionParams, priority), nil

}

func (opt *Operator) EntryBuilder(matchKeys []*MatchKeys, actionName string, params [][]byte, priority int32) *p4.TableEntry {
	matchFields, _ := opt.matchFieldBuilder(matchKeys)
	entryAction, _ := opt.entryActionBuilder(actionName, params)
	request := p4.TableEntry{
		Match:    matchFields,
		Action:   entryAction,
		TableId:  opt.tableID,
		Priority: priority,
	}
	return &request
}

func (opt *Operator) matchFieldBuilder(matchKeys []*MatchKeys) ([]*p4.FieldMatch, error) {
	// 判断一下 传入的参数的个数和table的key的个数是否一致
	if len(matchKeys) != len(opt.table.GetMatchFields()) {
		return nil, fmt.Errorf("传入的参数和table中的key数量不相同")
	}
	var returnValue []*p4.FieldMatch
	// 开始循环，首先解析matchKeys，然后根据match field的匹配类型，分别建立创建新的FieldMatch
	for index, field := range opt.table.GetMatchFields() {
		matchKey := matchKeys[index]
		// 检查忽略的表项
		if matchKey.DontCare {
			continue
		}
		// MatchField_EXACT
		if field.GetMatchType() == p4config.MatchField_EXACT {
			returnValue = append(returnValue, &p4.FieldMatch{
				FieldId: field.Id,
				FieldMatchType: &p4.FieldMatch_Exact_{
					Exact: &p4.FieldMatch_Exact{
						Value: matchKey.ExactValue,
					},
				},
			})
		}
		// MatchField_LPM
		if field.GetMatchType() == p4config.MatchField_LPM {
			returnValue = append(returnValue, &p4.FieldMatch{
				FieldId: field.Id,
				FieldMatchType: &p4.FieldMatch_Lpm{
					Lpm: &p4.FieldMatch_LPM{
						Value:     matchKey.LpmValue,
						PrefixLen: matchKey.LpmPrefixLen,
					},
				},
			})
		}
		// MatchField_TERNARY
		if field.GetMatchType() == p4config.MatchField_TERNARY {
			returnValue = append(returnValue, &p4.FieldMatch{
				FieldId: field.Id,
				FieldMatchType: &p4.FieldMatch_Ternary_{
					Ternary: &p4.FieldMatch_Ternary{
						Value: matchKey.TernaryValue,
						Mask:  matchKey.TernaryMask,
					},
				},
			})
		}
		// MatchField_RANGE
		if field.GetMatchType() == p4config.MatchField_RANGE {
			returnValue = append(returnValue, &p4.FieldMatch{
				FieldId: field.Id,
				FieldMatchType: &p4.FieldMatch_Range_{
					Range: &p4.FieldMatch_Range{
						Low:  matchKey.RangeLow,
						High: matchKey.RangeHigh,
					},
				},
			})
		}
	}

	return returnValue, nil
}

func (opt *Operator) entryActionBuilder(actionName string, params [][]byte) (*p4.TableAction, error) {
	// 0. 获得action实例
	if actionName == "" {
		return nil, nil
	}

	var action *p4config.Action
	var paramsSlice []*p4.Action_Param

	for _, item := range opt.p4info.GetActions() {
		if item.GetPreamble().Name == actionName {
			action = item
		}
	}
	// 1. 判断参数个数是否匹配
	if len(params) != len(action.GetParams()) {
		return nil, fmt.Errorf("传入的参数和action中定义的参数数量不相同")
	}

	for index, actionParam := range action.GetParams() {
		newParam := p4.Action_Param{
			ParamId: actionParam.Id,
			Value:   params[index],
		}
		paramsSlice = append(paramsSlice, &newParam)
	}

	tableActionAction := p4.TableAction_Action{
		Action: &p4.Action{
			ActionId: action.GetPreamble().Id,
			Params:   paramsSlice,
		},
	}

	return &p4.TableAction{
		Type: &tableActionAction,
	}, nil
}

func ActionProfileGroupCreateRequestBuilder(actionProfile *p4config.ActionProfile,
	actions []*p4.Action, groupID int, p4info p4config.P4Info) ([]*p4.ActionProfileMember, *p4.ActionProfileGroup, error) {
	//[preamble:<id:297808402 name:"hashed_selector" alias:"hashed_selector" > table_ids:39015874 with_selector:true size:1024 ]
	// 1. 声明基础变量
	var actionProfileMemberSlice []*p4.ActionProfileMember
	var actionProfileGroupMemberSlice []*p4.ActionProfileGroup_Member

	// 2. 生成ActionProfileMember对象的列表
	memberID := 0
	for _, action := range actions {
		actionProfileMemberSlice = append(actionProfileMemberSlice, &p4.ActionProfileMember{
			ActionProfileId: actionProfile.GetPreamble().GetId(),
			MemberId:        uint32(memberID),
			Action:          action,
		})
		memberID += 1
	}

	// 3. 生成actionProfileGroupMember对象
	for _, actionProfileMember := range actionProfileMemberSlice {
		actionProfileGroupMemberSlice = append(actionProfileGroupMemberSlice, &p4.ActionProfileGroup_Member{
			MemberId: actionProfileMember.GetMemberId(),
			Weight:   1,
		})
	}

	// 4. 生成ActionProfileGroup对象
	actionProfileGroup := p4.ActionProfileGroup{
		ActionProfileId: actionProfile.GetPreamble().GetId(),
		GroupId:         uint32(groupID),
		Members:         actionProfileGroupMemberSlice,
	}

	return actionProfileMemberSlice, &actionProfileGroup, nil
}

func ActionProfileGroupCreateRequestBuilder2(
	actionProfileName string,
	actionName string,
	actionParamsStringSlice [][]string,
	groupID int,
	p4info *p4config.P4Info) ([]*p4.ActionProfileMember, *p4.ActionProfileGroup, error) {

	var actionProfileConfig *p4config.ActionProfile
	var actionConfig *p4config.Action
	var actionSlice []*p4.Action

	for _, item := range p4info.GetActionProfiles() {
		if item.GetPreamble().GetName() == actionProfileName {
			actionProfileConfig = item
		}
	}

	for _, item := range p4info.GetActions() {
		if item.GetPreamble().GetName() == actionName {
			actionConfig = item
		}
	}

	for _, actionParamsString := range actionParamsStringSlice {
		fmt.Println(actionParamsString)
		var actionParamsSlice []*p4.Action_Param
		if len(actionParamsString) != len(actionConfig.GetParams()) {
			return nil, nil, fmt.Errorf("传入的参数数量和action中定义的参数数量不相同")
		}
		params, err := HexStringSliceToByteSlice(actionParamsString)
		if err != nil {
			return nil, nil, err
		}

		for index, actionParam := range actionConfig.GetParams() {
			newParam := p4.Action_Param{
				ParamId: actionParam.Id,
				Value:   params[index],
			}
			actionParamsSlice = append(actionParamsSlice, &newParam)
		}

		newAction := p4.Action{
			ActionId: actionConfig.GetPreamble().GetId(),
			Params:   actionParamsSlice,
		}
		actionSlice = append(actionSlice, &newAction)
	}

	return ActionProfileGroupCreateRequestBuilder(actionProfileConfig, actionSlice, groupID, *p4info)
}

func ActionProfileGroupCreateRequestBuilder3(
	actionProfileName string,
	actionName string,
	actionParams [][][]byte, // 填入多个action参数，每一个条记录的action参数是[][]byte, 多以多条action参数就是 [][][]byte
	groupID int,
	p4info *p4config.P4Info) ([]*p4.ActionProfileMember, *p4.ActionProfileGroup, error) {
	var actionProfileConfig *p4config.ActionProfile
	var actionConfig *p4config.Action
	var actionSlice []*p4.Action

	for _, item := range p4info.GetActionProfiles() {
		if item.GetPreamble().GetName() == actionProfileName {
			actionProfileConfig = item
		}
	}

	for _, item := range p4info.GetActions() {
		if item.GetPreamble().GetName() == actionName {
			actionConfig = item
		}
	}

	for _, actionParamsByteArray := range actionParams {
		var actionParamsSlice []*p4.Action_Param
		if len(actionParamsByteArray) != len(actionConfig.GetParams()) {
			return nil, nil, fmt.Errorf("传入的参数数量和action中定义的参数数量不相同")
		}
		for index, actionParam := range actionConfig.GetParams() {
			newParam := p4.Action_Param{
				ParamId: actionParam.Id,
				Value:   actionParamsByteArray[index],
			}
			actionParamsSlice = append(actionParamsSlice, &newParam)
		}

		newAction := p4.Action{
			ActionId: actionConfig.GetPreamble().GetId(),
			Params:   actionParamsSlice,
		}
		actionSlice = append(actionSlice, &newAction)
	}

	return ActionProfileGroupCreateRequestBuilder(actionProfileConfig, actionSlice, groupID, *p4info)

}

func ParseMatchKeysString(matchKeysString []string) ([]*MatchKeys, error) {
	LpmSep := "/"
	RangeSep := "..."
	TerrarySep := "&&&"
	var matchkeysSlice []*MatchKeys
	for _, item := range matchKeysString {
		// 检查是否忽略
		if strings.Compare(item, "_") == 0 {
			matchkeysSlice = append(matchkeysSlice, &MatchKeys{
				DontCare: true,
			})
			continue
		}
		// Lpm匹配
		if strings.Contains(item, LpmSep) {
			parts := strings.Split(item, LpmSep)
			returnPrefixLen, err := utils.StrToInt32(parts[1])
			var returnValue []byte
			if err != nil {
				return nil, fmt.Errorf("ParseMatchKeysLpm: prefixLen无法被解析: %v", err)
			}
			returnValue, err = EasyHexDecodeString2(parts[0])
			if err != nil {
				return nil, fmt.Errorf("ParseMatchKeysLpm: value %v 无法被解析: %v", parts[0], err)
			}
			matchkeysSlice = append(matchkeysSlice, &MatchKeys{
				LpmValue:     returnValue,
				LpmPrefixLen: returnPrefixLen,
			})
		} else if strings.Contains(item, TerrarySep) {
			parts := strings.Split(item, TerrarySep)
			returnValue, err := EasyHexDecodeString2(parts[0])
			if err != nil {
				return nil, fmt.Errorf("ParseMatchKeysTerrary: value %v 无法被解析: %v", parts[0], err)
			}
			returnMask, err := EasyHexDecodeString2(parts[1])
			if err != nil {
				return nil, fmt.Errorf("ParseMatchKeysTerrary: mask %v 无法被解析: %v", parts[1], err)
			}
			matchkeysSlice = append(matchkeysSlice, &MatchKeys{
				TernaryValue: returnValue,
				TernaryMask:  returnMask,
			})
		} else if strings.Contains(item, RangeSep) {
			parts := strings.Split(item, RangeSep)
			returnLow, err := EasyHexDecodeString2(parts[0])
			if err != nil {
				return nil, fmt.Errorf("ParseMatchKeysRange: Low无法被解析: %v", err)
			}
			returnHigh, err := EasyHexDecodeString2(parts[1])
			if err != nil {
				return nil, fmt.Errorf("ParseMatchKeysRange: High无法被解析: %v", err)
			}
			matchkeysSlice = append(matchkeysSlice, &MatchKeys{
				RangeLow:  returnLow,
				RangeHigh: returnHigh,
			})
		} else {
			returnValue, err := EasyHexDecodeString2(item)
			if err != nil {
				return nil, fmt.Errorf("ParseMatchKeysExact: %v 无法被解析: %v", item, err)
			}
			matchkeysSlice = append(matchkeysSlice, &MatchKeys{
				ExactValue: returnValue,
			})
		}
	}

	return matchkeysSlice, nil
}

func HexStringSliceToByteSlice(inputStringSlice []string) ([][]byte, error) {
	var returnByteSlice [][]byte
	for _, item := range inputStringSlice {
		if len(item)%2 != 0 { //检查变量的长度是否是偶数，如果不是，则在字符串前面添加一个 0 字符，使其长度变为偶数
			item = "0" + item
		}
		byteValue, err := hex.DecodeString(item)
		if err != nil {
			return nil, fmt.Errorf("HexStringSliceToByteSlice: string %v 无法被解析: %v", item, err)
		}
		returnByteSlice = append(returnByteSlice, byteValue)
	}
	return returnByteSlice, nil
}

func EasyHexDecodeString(hexStr string) ([]byte, error) {
	if len(hexStr)%2 != 0 {
		hexStr = "0" + hexStr
	}
	hexBytes, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, err
	}
	return hexBytes, nil
}

func EasyHexDecodeString2(hexOrIpStr string) ([]byte, error) {
	// FIXME: 添加 对MAC地址的解析
	ip := net.ParseIP(hexOrIpStr)
	if ip == nil {
		return EasyHexDecodeString(hexOrIpStr)
	} else {
		ip4 := ip.To4()
		if ip4 == nil {
			return nil, fmt.Errorf("convert failed, because it is not an IPv4 address")
		}
		return ip4, nil
	}

}

func ConvertIntToHexString(DecStr int) string {
	return strconv.FormatInt(int64(DecStr), 16)
}

/*
* 发送请求
 */
func InsertTableEntry(client p4.P4RuntimeClient, entry *p4.TableEntry) error {
	req := p4.WriteRequest{
		DeviceId: 1,
	}
	req.Updates = append(req.Updates, &p4.Update{
		Type: p4.Update_INSERT,
		Entity: &p4.Entity{
			Entity: &p4.Entity_TableEntry{
				TableEntry: entry,
			},
		},
	})
	res, err := client.Write(context.Background(), &req)
	if err != nil {
		fmt.Printf("添加表项失败：%s\n", err)
		return err
	}
	fmt.Printf("添加表项成功,respose := %v\n", res)
	return nil

}

func InsertActionProfileMember(client p4.P4RuntimeClient, member *p4.ActionProfileMember) error {
	req := p4.WriteRequest{ // !FIXME: 为什么这里是DeviceID：1， 需要将DeviceID 从配置拿出
		DeviceId: 1,
	}

	req.Updates = append(req.Updates, &p4.Update{
		Type: p4.Update_INSERT,
		Entity: &p4.Entity{
			Entity: &p4.Entity_ActionProfileMember{
				ActionProfileMember: member,
			},
		},
	})

	_, err := client.Write(context.Background(), &req)
	if err != nil {
		return fmt.Errorf("ActionProfileMember %v 添加失败 %v\n", member, err)
	}
	return nil
}

func InsertActionProfileGroup(client p4.P4RuntimeClient, group *p4.ActionProfileGroup) error {
	req := p4.WriteRequest{
		DeviceId: 1,
	}

	req.Updates = append(req.Updates, &p4.Update{
		Type: p4.Update_INSERT,
		Entity: &p4.Entity{
			Entity: &p4.Entity_ActionProfileGroup{
				ActionProfileGroup: group,
			},
		},
	})

	_, err := client.Write(context.Background(), &req)
	if err != nil {
		return fmt.Errorf("ActionProfileGroup %v 添加失败 %v\n", group, err)

	}
	return nil
}

func InsertActionProfileGroup2(client p4.P4RuntimeClient, memberSlice []*p4.ActionProfileMember, group *p4.ActionProfileGroup) error {
	for _, member := range memberSlice {
		err := InsertActionProfileMember(client, member)
		if err != nil {
			return err
		}
	}
	err := InsertActionProfileGroup(client, group)
	if err != nil {
		return err
	}
	return nil
}

func IfWildcardMatchKey(matchType string, first []byte, second []byte, third int32) *MatchKeys {
	var returnValue MatchKeys
	if first == nil {
		return &MatchKeys{
			DontCare: true,
		}
	}
	switch matchType {
	case "exact":
		returnValue = MatchKeys{
			ExactValue: first,
		}
	case "lpm":
		returnValue = MatchKeys{
			LpmValue:     first,
			LpmPrefixLen: third,
		}

	case "ternary":
		returnValue = MatchKeys{
			TernaryValue: first,
			TernaryMask:  second,
		}
	case "range":
		returnValue = MatchKeys{
			RangeLow:  first,
			RangeHigh: second,
		}
	}

	return &returnValue
}

func isBitSet(n int, i uint) bool {
	// 用于判断bit 是否为1
	return (n & (1 << i)) != 0
}

func Uint16ToByte(u [][]uint16) [][]byte {
	b := make([][]byte, len(u))
	for i, v := range u {
		b[i] = make([]byte, len(v)*2)
		for j, w := range v {
			binary.BigEndian.PutUint16(b[i][j*2:], w)
		}
	}
	return b
}
