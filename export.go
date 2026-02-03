package iplist

import (
	"bufio"
	"io"
	"strconv"
	"strings"
)

// ExportCountryTSV writes the full CountryID -> (code,name) mapping table.
//
// Output format: tab-separated values with a header row:
//   country_id\tcountry_code\tcountry_name
func (db *DB) ExportCountryTSV(w io.Writer) error {
	if db == nil || db.v4 == nil {
		return ErrInvalidDB
	}
	bw := bufio.NewWriter(w)
	if _, err := bw.WriteString("country_id\tcountry_code\tcountry_name\n"); err != nil {
		return err
	}
	var line []byte
	for i := range db.v4.countryLabels {
		id := uint32(i)
		code, name := db.v4.countryLabel(id)
		if code == "" {
			continue
		}
		line = strconv.AppendUint(line[:0], uint64(id), 10)
		line = append(line, '\t')
		line = append(line, code...)
		line = append(line, '\t')
		line = append(line, name...)
		line = append(line, '\n')
		if _, err := bw.Write(line); err != nil {
			return err
		}
	}
	return bw.Flush()
}

// ExportCNProvinceTSV writes the full CNProvinceID -> (code,name) mapping table.
//
// CNProvinceID values are indices into the CN label table.
// Output header:
//   cn_province_id\tcn_province_code\tcn_province_name
func (db *DB) ExportCNProvinceTSV(w io.Writer) error {
	return db.exportCNTSV(w, true)
}

// ExportCNCityTSV writes the full CNCityID -> (code,name) mapping table.
//
// CNCityID values are indices into the CN label table.
// Output header:
//   cn_city_id\tcn_city_code\tcn_city_name
func (db *DB) ExportCNCityTSV(w io.Writer) error {
	return db.exportCNTSV(w, false)
}

func (db *DB) exportCNTSV(w io.Writer, province bool) error {
	if db == nil || db.v4 == nil {
		return ErrInvalidDB
	}
	bw := bufio.NewWriter(w)
	head := "cn_city_id\tcn_city_code\tcn_city_name\n"
	if province {
		head = "cn_province_id\tcn_province_code\tcn_province_name\n"
	}
	if _, err := bw.WriteString(head); err != nil {
		return err
	}

	var line []byte
	for i := range db.v4.cnLabels {
		id := uint32(i)
		code, name := db.v4.cnLabel(id)
		if code == "" {
			continue
		}
		isProv := strings.HasSuffix(code, "0000")
		isCity := strings.HasSuffix(code, "00") && !isProv
		if province {
			if !isProv {
				continue
			}
		} else {
			if !isCity {
				continue
			}
		}

		line = strconv.AppendUint(line[:0], uint64(id), 10)
		line = append(line, '\t')
		line = append(line, code...)
		line = append(line, '\t')
		line = append(line, name...)
		line = append(line, '\n')
		if _, err := bw.Write(line); err != nil {
			return err
		}
	}
	return bw.Flush()
}

// ExportProviderTSV writes the full ProviderID -> (key,name,kind) mapping table.
//
// Output header:
//   provider_id\tprovider_key\tprovider_name\tprovider_kind
func (db *DB) ExportProviderTSV(w io.Writer) error {
	if db == nil || db.v4 == nil {
		return ErrInvalidDB
	}
	bw := bufio.NewWriter(w)
	if _, err := bw.WriteString("provider_id\tprovider_key\tprovider_name\tprovider_kind\n"); err != nil {
		return err
	}
	var line []byte
	for i := range db.v4.providerLabels {
		id := uint32(i)
		key, name, kind := db.v4.providerLabel(id)
		if key == "" {
			continue
		}
		line = strconv.AppendUint(line[:0], uint64(id), 10)
		line = append(line, '\t')
		line = append(line, key...)
		line = append(line, '\t')
		line = append(line, name...)
		line = append(line, '\t')
		line = strconv.AppendUint(line, uint64(kind), 10)
		line = append(line, '\n')
		if _, err := bw.Write(line); err != nil {
			return err
		}
	}
	return bw.Flush()
}
