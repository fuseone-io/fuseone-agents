package connectortools

import (
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// postgresJSONRow preserves database meaning rather than serialising pgx's
// incidental Go representation. Exact and identifier-like types use their
// PostgreSQL text codec; JSON stays structured and bytea stays base64.
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
		if dataType, ok := typeMap.TypeForOID(field.DataTypeOID); ok {
			value, err := postgresJSONValue(typeMap, dataType, field.Format, src)
			if err != nil {
				return nil, err
			}
			values[i] = value
			continue
		}
		if field.Format == pgtype.TextFormatCode {
			values[i] = string(src)
		} else {
			values[i] = append([]byte(nil), src...)
		}
	}
	return json.Marshal(values)
}

func postgresJSONValue(
	typeMap *pgtype.Map, dataType *pgtype.Type, format int16, src []byte,
) (any, error) {
	if postgresTextualOID(dataType.OID) {
		return dataType.Codec.DecodeDatabaseSQLValue(typeMap, dataType.OID, format, src)
	}
	value, err := dataType.Codec.DecodeValue(typeMap, dataType.OID, format, src)
	if err != nil {
		return nil, err
	}
	elementOID, ok := postgresTextualArrayElement(dataType.OID)
	if !ok {
		return value, nil
	}
	elementType, ok := typeMap.TypeForOID(elementOID)
	if !ok {
		return nil, errors.New("connector: postgres array element type is unknown")
	}
	return postgresTextualArray(typeMap, elementType, value)
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

func postgresTextualOID(oid uint32) bool {
	switch oid {
	case pgtype.UUIDOID, pgtype.NumericOID,
		pgtype.InetOID, pgtype.CIDROID,
		pgtype.MacaddrOID, pgtype.Macaddr8OID,
		pgtype.BitOID, pgtype.VarbitOID, pgtype.IntervalOID:
		return true
	default:
		return false
	}
}

func postgresTextualArrayElement(oid uint32) (uint32, bool) {
	switch oid {
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
