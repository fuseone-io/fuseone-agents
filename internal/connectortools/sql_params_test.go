package connectortools

import (
	"math"
	"testing"
)

func TestSQLArguments_keepJSONNumbersUntilTheirDeclaredTypeIsKnown(t *testing.T) {
	t.Parallel()

	args, ok := decodeSQLRunArgs([]byte(`{
		"template_id":"numeric_report",
		"parameters":{"row_id":9007199254740993,"ratio":1.25}
	}`))
	if !ok {
		t.Fatal("valid numeric arguments were refused")
	}
	bound, err := bindParameters(SQLTemplate{
		ID: "numeric_report",
		Parameters: []SQLParameter{
			{Name: "row_id", Type: SQLParamInteger},
			{Name: "ratio", Type: SQLParamNumber},
		},
	}, args.Parameters)
	if err != nil {
		t.Fatalf("bindParameters: %v", err)
	}
	if bound[0] != int64(9007199254740993) || bound[1] != 1.25 {
		t.Fatalf("bound = %#v, want an exact integer and a finite number", bound)
	}
}

func TestSQLArguments_refuseNonFiniteNumbers(t *testing.T) {
	t.Parallel()

	for name, value := range map[string]any{
		"nan": math.NaN(), "positive infinity": math.Inf(1), "text infinity": "Inf",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := bindParameters(SQLTemplate{
				ID:         "numeric_report",
				Parameters: []SQLParameter{{Name: "ratio", Type: SQLParamNumber}},
			}, map[string]any{"ratio": value})
			if err == nil {
				t.Fatalf("%v was accepted as a database number", value)
			}
		})
	}
}
