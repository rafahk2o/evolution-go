package instance_service

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStatusStructExposesInstanceName(t *testing.T) {
	value, err := json.Marshal(StatusStruct{InstanceName: "Atendimento", Name: "Teste"})
	if err != nil {
		t.Fatalf("marshal status: %v", err)
	}

	encoded := string(value)
	if !strings.Contains(encoded, `"InstanceName":"Atendimento"`) {
		t.Fatalf("status does not expose instance name: %s", encoded)
	}
}
