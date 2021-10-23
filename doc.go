package contractgen

import (
	"fmt"
	"io"

	"github.com/pelletier/go-toml"
)

// Para contains the text template for a paragraph
type Para struct {
	Text    string
	Options []string
	Tag     string
}

// Options for paragraph formatting
const (
	OptCenter            string = "Center"
	OptNormalSpacing            = "NormalSpacing"
	OptBeforeOnlySpacing        = "BeforeOnlySpacing"
	OptAfterOnlySpacing         = "AfterOnlySpacing"
	OptUnderline                = "Underline"
	OptNumberingLevel1          = "NumberingLevel1"
	OptNumberingLevel2          = "NumberingLevel2"
	OptNumberingLevel3          = "NumberingLevel3"
	OptNumberingGroup01         = "NumberingGroup01"
	OptNumberingGroup02         = "NumberingGroup02"
	OptNumberingGroup03         = "NumberingGroup03"
	OptNumberingGroup04         = "NumberingGroup04"
	OptNumberingGroup05         = "NumberingGroup05"
	OptNumberingGroup06         = "NumberingGroup06"
	OptNumberingGroup07         = "NumberingGroup07"
	OptNumberingGroup08         = "NumberingGroup08"
	OptNumberingGroup09         = "NumberingGroup09"
	OptNumberingGroup10         = "NumberingGroup10"
)

// ClientTemplate for parsing client contract
type ClientTemplate struct {
	Fixed                              []Para
	ConsultantArrangementTimesheet     map[string][]Para
	ConsultantArrangementInduction     map[string][]Para
	ConsultantArrangementPaidLeave     map[string][]Para
	ConsultantArrangementReimbursement map[string][]Para
	Commercial                         map[string][]Para
}

// LoadClientTemplate loads a client template from an io.Reader
func LoadClientTemplate(r io.Reader) (*ClientTemplate, error) {
	clientTemplate := ClientTemplate{}

	d := toml.NewDecoder(r)
	if err := d.Decode(&clientTemplate); err != nil {
		return nil, fmt.Errorf("error in LoadClientTemplate decoding toml: %s", err)
	}

	return &clientTemplate, nil
}
