package cwmp

import (
	"encoding/xml"
	"fmt"
	"strings"
	"testing"
)

// --- Bug-catching: XML injection ---

func TestGetParameterValues_EscapesXMLSpecialChars(t *testing.T) {
	result := GetParameterValues(`<script>alert(1)</script>`)
	if strings.Contains(result, `<script>alert(1)</script>`) {
		t.Error("XML injection: raw <script> tag present in output, expected escaped")
	}
	if !strings.Contains(result, `&lt;script&gt;`) {
		t.Error("Expected escaped &lt;script&gt; in output")
	}
}

func TestSetParameterValues_EscapesAmpersand(t *testing.T) {
	result := SetParameterValues("Device.Name", "AT&T")
	if strings.Contains(result, `>AT&T<`) {
		t.Error("XML injection: raw ampersand in value, expected &amp;")
	}
	if !strings.Contains(result, `AT&amp;T`) {
		t.Error("Expected AT&amp;T in output")
	}
}

func TestSetParameterMultiValues_EscapesAllEntries(t *testing.T) {
	data := map[string]string{
		"Device.<Evil>": "val&ue",
	}
	result := SetParameterMultiValues(data)
	if strings.Contains(result, `<Evil>`) {
		t.Error("XML injection: raw <Evil> in key")
	}
	if strings.Contains(result, `>val&ue<`) {
		t.Error("XML injection: raw & in value")
	}
}

func TestGetParameterNames_EscapesSpecialChars(t *testing.T) {
	result := GetParameterNames(`<path>`, 0)
	if strings.Contains(result, `<ParameterPath><path>`) {
		t.Error("XML injection: raw <path> tag present")
	}
}

func TestDownload_EscapesURLParams(t *testing.T) {
	result := Download("1", "http://fw.example.com/v1?token=abc&sig=123", "user", "pass", "1024")
	if strings.Contains(result, `?token=abc&sig=`) && !strings.Contains(result, `&amp;sig=`) {
		t.Error("XML injection: raw & in URL parameter, expected &amp;")
	}
}

func TestGetParameterMultiValues_EscapesAll(t *testing.T) {
	result := GetParameterMultiValues([]string{`"<injected>"`, `normal.path.`})
	if strings.Contains(result, `<injected>`) {
		t.Error("XML injection: raw angle brackets in multi-value param")
	}
}

func TestInformResponse_EscapesMustUnderstand(t *testing.T) {
	result := InformResponse(`"><evil/>`)
	if strings.Contains(result, `<evil/>`) {
		t.Error("XML injection: raw tag in mustUnderstand header")
	}
}

// --- Bug-catching: Namespace typo ---

func TestChangeDuState_CorrectNamespace(t *testing.T) {
	ops := []fmt.Stringer{
		&InstallOpStruct{Url: "http://example.com", Uuid: "123", Username: "u", Password: "p", ExecutionEnvironment: "env"},
	}
	result := ChangeDuState(ops)
	if strings.Contains(result, "cmwp:") {
		t.Error("Namespace typo: found 'cmwp:' instead of 'cwmp:' in ChangeDuState")
	}
	if !strings.Contains(result, "cwmp:ChangeDUState") {
		t.Error("Expected 'cwmp:ChangeDUState' in output")
	}
}

// --- Bug-catching: Malformed XML ---

func TestCancelTransfer_ValidClosingTag(t *testing.T) {
	result := CancelTransfer()
	if strings.Contains(result, "<cwmp:CancelTransfer/>") {
		t.Error("Malformed XML: self-closing tag used as closing tag")
	}
	if !strings.Contains(result, "</cwmp:CancelTransfer>") {
		t.Error("Expected proper closing tag </cwmp:CancelTransfer>")
	}
}

// --- Bug-catching: All messages must be well-formed XML ---

func TestAllMessages_AreWellFormedXML(t *testing.T) {
	messages := map[string]string{
		"GetParameterValues":      GetParameterValues("Device.DeviceInfo."),
		"GetParameterMultiValues": GetParameterMultiValues([]string{"Device.DeviceInfo.", "Device.WiFi."}),
		"SetParameterValues":      SetParameterValues("Device.WiFi.SSID.1.SSID", "TestNetwork"),
		"SetParameterMultiValues": SetParameterMultiValues(map[string]string{"Device.WiFi.SSID.1.SSID": "Test"}),
		"GetParameterNames":       GetParameterNames("Device.", 1),
		"FactoryReset":            FactoryReset(),
		"Download":                Download("1", "http://example.com/fw.bin", "admin", "pass", "1024"),
		"CancelTransfer":          CancelTransfer(),
		"ScheduleDownload":        ScheduleDownload("1", "http://example.com/fw.bin", "admin", "pass", "1024", nil),
		"ChangeDuState":           ChangeDuState(nil),
		"InformResponse":          InformResponse("1234"),
		"InformResponseEmpty":     InformResponse(""),
		"Inform":                  Inform("SN123456"),
	}

	for name, msg := range messages {
		t.Run(name, func(t *testing.T) {
			var parsed interface{}
			err := xml.Unmarshal([]byte(msg), &parsed)
			if err != nil {
				t.Errorf("message %s is not well-formed XML: %v", name, err)
			}
		})
	}
}

// --- Happy-path safety net ---

func TestGetParameterValues_NormalInput(t *testing.T) {
	result := GetParameterValues("Device.DeviceInfo.")
	if !strings.Contains(result, "soap:Envelope") {
		t.Error("Missing SOAP envelope")
	}
	if !strings.Contains(result, "cwmp:GetParameterValues") {
		t.Error("Missing cwmp:GetParameterValues element")
	}
	if !strings.Contains(result, "Device.DeviceInfo.") {
		t.Error("Missing parameter path in output")
	}
}

func TestSetParameterValues_NormalInput(t *testing.T) {
	result := SetParameterValues("Device.WiFi.SSID.1.SSID", "MyNetwork")
	if !strings.Contains(result, "cwmp:SetParameterValues") {
		t.Error("Missing cwmp:SetParameterValues element")
	}
	if !strings.Contains(result, "<Name>Device.WiFi.SSID.1.SSID</Name>") {
		t.Error("Missing parameter name")
	}
	if !strings.Contains(result, "<Value>MyNetwork</Value>") {
		t.Error("Missing parameter value")
	}
}

func TestDownload_NormalInput(t *testing.T) {
	result := Download("1", "http://example.com/fw.bin", "admin", "secret", "2048")
	if !strings.Contains(result, "cwmp:Download") {
		t.Error("Missing cwmp:Download element")
	}
	if !strings.Contains(result, "<FileType>1</FileType>") {
		t.Error("Missing FileType")
	}
	if !strings.Contains(result, "<URL>http://example.com/fw.bin</URL>") {
		t.Error("Missing URL")
	}
	if !strings.Contains(result, "<Username>admin</Username>") {
		t.Error("Missing Username")
	}
}

func TestFactoryReset_ProducesValidSOAP(t *testing.T) {
	result := FactoryReset()
	if !strings.Contains(result, "cwmp:FactoryReset") {
		t.Error("Missing cwmp:FactoryReset element")
	}
}

func TestInform_ParsesCorrectly(t *testing.T) {
	xmlData := Inform("SN123456")
	var envelope SoapEnvelope
	if err := xml.Unmarshal([]byte(xmlData), &envelope); err != nil {
		t.Fatalf("Failed to parse Inform XML: %v", err)
	}

	var inform CWMPInform
	if err := xml.Unmarshal([]byte(xmlData), &inform); err != nil {
		t.Fatalf("Failed to parse Inform body: %v", err)
	}
	if inform.DeviceId.SerialNumber != "SN123456" {
		t.Errorf("Expected SerialNumber SN123456, got %s", inform.DeviceId.SerialNumber)
	}
	if inform.GetDataModelType() != "TR098" {
		t.Errorf("Expected TR098, got %s", inform.GetDataModelType())
	}
	if inform.GetSoftwareVersion() != "4.0.8.17785" {
		t.Errorf("Expected 4.0.8.17785, got %s", inform.GetSoftwareVersion())
	}
	connReq := inform.GetConnectionRequest()
	if !strings.Contains(connReq, "SN123456") {
		t.Errorf("Connection request URL should contain serial, got %s", connReq)
	}
}

func TestGetDataModelType_TR181(t *testing.T) {
	inform := CWMPInform{
		ParameterList: []ParameterValueStruct{
			{Name: "Device.DeviceInfo.SoftwareVersion", Value: "1.0"},
		},
	}
	if inform.GetDataModelType() != "TR181" {
		t.Errorf("Expected TR181, got %s", inform.GetDataModelType())
	}
}

func TestGetDataModelType_EmptyParameterList_DoesNotPanic(t *testing.T) {
	inform := CWMPInform{
		ParameterList: []ParameterValueStruct{},
	}
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("GetDataModelType panicked on empty ParameterList: %v", r)
		}
	}()
	_ = inform.GetDataModelType()
}

func TestParamTypeIsWritable(t *testing.T) {
	if !ParamTypeIsWritable("1") {
		t.Error("Expected '1' to be writable")
	}
	if ParamTypeIsWritable("0") {
		t.Error("Expected '0' to not be writable")
	}
}
