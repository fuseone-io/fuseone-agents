package connectortools

import (
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// postgresJSONRow preserves database meaning rather than serialising pgx's
// incidental Go representation. PostgreSQL text is the closed default. Only
// types whose JSON representation is deliberately named below become native
// JSON values; adding a pgx codec cannot change a governed result by accident.
func postgresJSONRow(rows pgx.Rows) (json.RawMessage, error) {
	fields := rows.FieldDescriptions()
	raw := rows.RawValues()
	if len(fields) != len(raw) || rows.Conn() == nil {
		return nil, errors.New("connector: postgres returned an invalid row shape")
	}
	typeMap := rows.Conn().TypeMap()
	values := make([]any, len(raw))
	for i, src := range raw {
		if src == nil {
			continue
		}
		field := fields[i]
		if field.Format != pgtype.TextFormatCode {
			return nil, errors.New("connector: postgres returned a non-text value")
		}
		value, err := postgresJSONValue(typeMap, field.DataTypeOID, src)
		if err != nil {
			return nil, err
		}
		values[i] = value
	}
	return json.Marshal(values)
}

func postgresJSONValue(
	typeMap *pgtype.Map, oid uint32, src []byte,
) (any, error) {
	if oid == pgtype.JSONOID || oid == pgtype.JSONBOID {
		if !json.Valid(src) {
			return nil, errors.New("connector: postgres returned invalid JSON")
		}
		return append(json.RawMessage(nil), src...), nil
	}
	dataType, ok := typeMap.TypeForOID(oid)
	if !ok {
		return string(src), nil
	}
	if !postgresStructuredOID(oid) {
		elementOID, textualArray := postgresTextualArrayElement(oid)
		if !textualArray {
			return string(src), nil
		}
		value, err := dataType.Codec.DecodeValue(typeMap, oid, pgtype.TextFormatCode, src)
		if err != nil {
			return nil, err
		}
		elementType, ok := typeMap.TypeForOID(elementOID)
		if !ok {
			return nil, errors.New("connector: postgres array element type is unknown")
		}
		return postgresTextualArray(typeMap, elementType, value)
	}
	value, err := dataType.Codec.DecodeValue(typeMap, oid, pgtype.TextFormatCode, src)
	if err != nil {
		return nil, err
	}
	return value, nil
}

func postgresTextualArray(
	typeMap *pgtype.Map, elementType *pgtype.Type, value any,
) (any, error) {
	values, ok := value.([]any)
	if !ok {
		return nil, errors.New("connector: postgres returned an invalid array")
	}
	result := make([]any, len(values))
	for i, item := range values {
		if nested, ok := item.([]any); ok {
			converted, err := postgresTextualArray(typeMap, elementType, nested)
			if err != nil {
				return nil, err
			}
			result[i] = converted
			continue
		}
		if item == nil {
			continue
		}
		encoded, err := typeMap.Encode(
			elementType.OID, pgtype.TextFormatCode, item, nil)
		if err != nil {
			return nil, err
		}
		result[i] = string(encoded)
	}
	return result, nil
}

func postgresStructuredOID(oid uint32) bool {
	switch oid {
	case pgtype.BoolOID, pgtype.Int2OID, pgtype.Int4OID, pgtype.ByteaOID,
		pgtype.BoolArrayOID, pgtype.Int2ArrayOID, pgtype.Int4ArrayOID,
		pgtype.TextArrayOID, pgtype.VarcharArrayOID, pgtype.BPCharArrayOID,
		pgtype.NameArrayOID, pgtype.ByteaArrayOID:
		return true
	default:
		return false
	}
}

func postgresTextualArrayElement(oid uint32) (uint32, bool) {
	switch oid {
	case pgtype.Int8ArrayOID:
		return pgtype.Int8OID, true
	case pgtype.UUIDArrayOID:
		return pgtype.UUIDOID, true
	case pgtype.NumericArrayOID:
		return pgtype.NumericOID, true
	case pgtype.InetArrayOID:
		return pgtype.InetOID, true
	case pgtype.CIDRArrayOID:
		return pgtype.CIDROID, true
	case pgtype.MacaddrArrayOID:
		return pgtype.MacaddrOID, true
	case pgtype.BitArrayOID:
		return pgtype.BitOID, true
	case pgtype.VarbitArrayOID:
		return pgtype.VarbitOID, true
	case pgtype.IntervalArrayOID:
		return pgtype.IntervalOID, true
	default:
		return 0, false
	}
}
