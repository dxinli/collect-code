package config

import "strconv"

// Validation rule identifiers. They are part of config-check's output
// contract: every diagnostic must carry one, and golden tests assert them.
const (
	RuleSyntax                = "V01" // YAML syntax / multiple documents
	RuleEmptyDocument         = "V02" // empty file or null document
	RuleUnknownKey            = "V03" // unknown key at any nesting level
	RuleDuplicateKey          = "V04" // duplicate mapping key
	RuleTypeMismatch          = "V05" // value cannot be decoded into the field
	RuleFile                  = "V06" // missing/unreadable file or directory
	RuleDuplicateProfile      = "V07" // same profile name in two files
	RuleRequiredConfig        = "V10" // config.yaml required fields
	RuleRequiredLink          = "V11" // link required fields/transport
	RuleRequiredDevice        = "V12" // device required fields/address
	RuleRequiredProfile       = "V13" // profile name
	RuleRequiredPointKey      = "V14" // point key
	RuleRequiredServiceID     = "V15" // service_id
	RuleRequiredEventID       = "V16" // event_id
	RuleUniqueLinkID          = "V20"
	RuleUniqueDeviceSN        = "V21"
	RuleUniquePointKey        = "V22"
	RuleUniqueServiceID       = "V23"
	RuleUniqueEventID         = "V24"
	RuleProfileRef            = "V30"
	RuleLinkRef               = "V31"
	RuleWritePointMissing     = "V32"
	RuleWritePointReadOnly    = "V33"
	RuleVerifyPointMissing    = "V34"
	RuleVerifyPointUnreadable = "V35"
	RuleMinMax                = "V40"
	RuleScale                 = "V41"
	RulePollInterval          = "V42"
	RuleFreshnessWindow       = "V43"
	RuleOfflineIntervals      = "V44"
	RuleQoS                   = "V45"
	RuleDriver                = "V50"
	RuleDriverTransport       = "V51"
	RuleParity                = "V52"
	RuleAccess                = "V53"
	RuleHealthPort            = "V60"
	RuleCredential            = "V61"
	RuleLogLevel              = "V62"
	RuleLogFormat             = "V63"
	RuleDeviceTypeMismatch    = "V70" // device/profile device_type consistency
	RuleDeviceTypeEnum        = "V71" // device/profile device_type enum
	RuleEventTriggerPoint     = "V72" // point_changed trigger point exists and is readable
	RuleWriteFromParam        = "V73" // writes[].from references a declared service param
)

// ruleNumber extracts the numeric part of a rule ID for deterministic
// ordering. Unknown IDs sort last.
func ruleNumber(id string) int {
	if len(id) < 2 || id[0] != 'V' {
		return 1 << 30
	}
	n, err := strconv.Atoi(id[1:])
	if err != nil {
		return 1 << 30
	}
	return n
}
