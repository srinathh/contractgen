package contractgen // import "bitbucket.org/srinathh/contractgen"

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"os"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"

	"google.golang.org/appengine"
	"google.golang.org/appengine/log"

	"baliance.com/gooxml/color"
	"baliance.com/gooxml/document"
	"baliance.com/gooxml/measurement"
	"baliance.com/gooxml/schema/soo/wml"
)

func init() {
	http.HandleFunc("/genclientdoc", genclientdoc)
}

type contextKey string

var (
	uuidKey = contextKey("uuid")
)

func writeError(c context.Context, w http.ResponseWriter, err error) string {
	uuid := c.Value(uuidKey).(string)
	log.Errorf(c, "Error in Request: %s:%+v", uuid, err)
	http.Error(w, fmt.Sprintf("An error occured. Please contact admin with this error code: %s", uuid), http.StatusInternalServerError)
	return uuid
}

func validateInput(input string, valid []string) (string, bool) {
	for _, item := range valid {
		if input == item {
			return input, true
		}
	}
	return "", false
}

var (
	validWorkLocations = []string{"Home", "Office"}
	validYesNo         = []string{"Yes", "No"}
	validContractTypes = []string{"Monthly", "OutputBased", "Hours"}
)

func genclientdoc(w http.ResponseWriter, r *http.Request) {

	// generate a unique key for this requeist
	uuid := uuid.New().String()
	c := context.WithValue(appengine.NewContext(r), uuidKey, uuid)

	// log the request
	b, err := httputil.DumpRequest(r, true)
	if err != nil {
		writeError(c, w, errors.Wrap(err, "error logging request"))
		return
	}
	log.Infof(c, "Request: %s: %s", uuid, string(b))

	// For developemnt use. We load client template in the request handler
	// for production use, load it in init only once on app startup
	fil, err := os.Open("clienttemplate.toml")
	if err != nil {
		writeError(c, w, errors.Wrap(err, "error opening client template"))
		return
	}
	clientTemplate, err := LoadClientTemplate(fil)
	if err != nil {
		writeError(c, w, errors.Wrap(err, "error parsing client template"))
		return
	}

	if err := r.ParseForm(); err != nil {
		writeError(c, w, errors.Wrap(err, "error parsing input form data"))
		return
	}

	// parameters for variable substitution from the form. We are not validating these right now
	params := map[string]string{}
	params["ClientEntityName"] = r.FormValue("ClientEntityName")
	params["ClientAddress"] = r.FormValue("ClientAddress")
	params["ConsultantName"] = r.FormValue("ConsultantName")
	params["RoleName"] = r.FormValue("RoleName")
	params["RoleDescription"] = r.FormValue("RoleDescription")
	params["NatureOfTravel"] = r.FormValue("NatureOfTravel")

	if workLocation, ok := validateInput(r.FormValue("WorkLocation"), validWorkLocations); ok {
		params["WorkLocation"] = workLocation
	} else {
		writeError(c, w, fmt.Errorf("invalid work location: %s", workLocation))
		return
	}

	// contract date validation
	contractDate, err := time.Parse("2006-01-02", r.FormValue("ContractDate"))
	if err != nil {
		writeError(c, w, errors.Wrap(err, "error parsing Contract Date"))
		return
	}
	params["ContractDate"] = contractDate.Format("_2 January 2006")

	// contract start date validation and used to calculate end date
	contractStartDate, err := time.Parse("2006-01-02", r.FormValue("ContractStartDate"))
	if err != nil {
		writeError(c, w, errors.Wrap(err, "error parsing Contract Start Date"))
		return
	}
	params["ContractStartDate"] = contractStartDate.Format("_2 January 2006")

	// contract duration validation - used to calculate end date and leaves allowed
	contractDuration, err := strconv.Atoi(r.FormValue("ContractDuration"))
	if err != nil {
		writeError(c, w, errors.Wrap(err, "error parsing contract duration as number"))
		return
	}
	params["ContractDuration"] = strconv.Itoa(contractDuration)

	// end date calculation. one day after X months
	params["ContractEndDate"] = contractStartDate.AddDate(0, contractDuration, -1).Format("_2 January 2006")

	// hours per day
	hoursPerDay, err := strconv.Atoi(r.FormValue("HoursPerDay"))
	if err != nil {
		writeError(c, w, errors.Wrap(err, "error parsing hours per day as number"))
		return
	}
	params["HoursPerDay"] = strconv.Itoa(hoursPerDay)

	// days per week
	daysPerWeek, err := strconv.Atoi(r.FormValue("DaysPerWeek"))
	if err != nil {
		writeError(c, w, errors.Wrap(err, "error parsing days per week as number"))
		return
	}
	params["DaysPerWeek"] = strconv.Itoa(daysPerWeek)

	// if we don't have hours per month entered, then we calculate a range with +/- 5 hours
	if r.FormValue("HoursPerMonth") == "" {
		mid := hoursPerDay * daysPerWeek * 4
		params["HoursPerMonth"] = fmt.Sprintf("%d to %d", mid-5, mid+5)
	} else {
		params["HoursPerMonth"] = r.FormValue("HoursPerMonth")
	}

	// log all the final parameters
	log.Infof(c, "Request: %s: Configuration: %v", uuid, params)

	// NOW WE START BUILDING THE LINES IN THE CONTRACT

	// Add the preamble lines
	docContent := append([]Para{}, clientTemplate.Fixed...)

	// validate the client timesheet input
	clientTimesheet := r.FormValue("ClientTimesheet")
	validClientTimesheetEntries := map[string]bool{"Yes": true, "No": true}
	if _, ok := validClientTimesheetEntries[clientTimesheet]; !ok {
		writeError(c, w, fmt.Errorf("invalid Client Timesheet entry received: %s. Must be Yes or No", r.FormValue("ClientTimesheet")))
		return
	}
	// pick the para & insert it into clientWorkingArrangements
	timeSheetClauses := clientTemplate.ConsultantArrangementTimesheet[clientTimesheet]
	docContent, err = insertParas(docContent, timeSheetClauses, "HoursScheduleClause")
	if err != nil {
		writeError(c, w, errors.Wrap(err, "error in timesheet clause"))
		return
	}

	// validate the induction industry stuff
	inductionVertical := r.FormValue("InductionVertical")
	validInductionVerticals := map[string]bool{"Graphic": true, "Technology": true, "Marketing": true, "Other": true}
	if _, ok := validInductionVerticals[inductionVertical]; !ok {
		writeError(c, w, fmt.Errorf("invalid induction vertical received: %s", r.FormValue("InductionVertical")))
		return
	}
	inductionClauses := append([]Para{}, clientTemplate.ConsultantArrangementInduction["Standard"]...)
	inductionClauses = append(inductionClauses, clientTemplate.ConsultantArrangementInduction[inductionVertical]...)
	docContent, err = insertParas(docContent, inductionClauses, "MainInductionClause")
	if err != nil {
		writeError(c, w, errors.Wrap(err, "error in inductionclause"))
		return
	}

	// face to face induction cluase
	inductionF2F := r.FormValue("InductionF2F")
	validInductionF2FEntries := map[string]bool{"Required": true, "Not Required": true}
	if _, ok := validInductionF2FEntries[inductionF2F]; !ok {
		writeError(c, w, fmt.Errorf("invalid induction f2f entries: %s", inductionF2F))
		return
	}
	// ClientReportingClause
	if inductionF2F == "Required" {
		docContent, err = insertParas(docContent, clientTemplate.ConsultantArrangementInduction["Face2Face"], "ClientReportingClause")
		if err != nil {
			writeError(c, w, fmt.Errorf("could not insert client working arrangement caluse"))
			return
		}
	}

	// put together the leaves clause. First read thel eaves per month
	// and then pick the right wording
	leavesPerMonth, err := strconv.Atoi(r.FormValue("LeavesPerMonth"))
	if err != nil {
		writeError(c, w, errors.Wrap(err, "leaves per month should be a number"))
		return
	}
	// leave days calculation at 2 per month
	params["LeaveDays"] = strconv.Itoa(contractDuration * leavesPerMonth)
	nationalHolidayClause := clientTemplate.ConsultantArrangementPaidLeave["WithNationalHolidayClause"]
	reqNationalHolidayClause, ok := validateInput(r.FormValue("NationalHolidayClause"), validYesNo)
	if !ok {
		writeError(c, w, fmt.Errorf("received invalid input for national holiday cluase: %s", nationalHolidayClause))
		return
	}
	if reqNationalHolidayClause == "No" {
		nationalHolidayClause = clientTemplate.ConsultantArrangementPaidLeave["WithoutNationalHolidayClause"]
	}
	docContent, err = insertParas(docContent, nationalHolidayClause, "ContractEndDate")
	if err != nil {
		writeError(c, w, errors.Wrap(err, "could not insert national holiday cluase"))
		return
	}

	reimbursements := []Para{}
	travelReimbursement, ok := validateInput(r.FormValue("TravelReimbursement"), validYesNo)
	if !ok {
		writeError(c, w, fmt.Errorf("received invalid input for travel reimbursement:%s", travelReimbursement))
		return
	}
	if travelReimbursement == "Yes" {
		reimbursements = append(reimbursements, clientTemplate.ConsultantArrangementReimbursement["Travel"]...)
	}
	phoneReimbursement, ok := validateInput(r.FormValue("PhoneReimbursement"), validYesNo)
	if !ok {
		writeError(c, w, fmt.Errorf("received invalid input for phone reimbursement: %s", phoneReimbursement))
		return
	}
	if phoneReimbursement == "Yes" {
		reimbursements = append(reimbursements, clientTemplate.ConsultantArrangementReimbursement["Phone"]...)
	}
	docContent, err = insertParas(docContent, reimbursements, "BestPractices")
	if err != nil {
		writeError(c, w, errors.Wrap(err, "could not insert reimbursements"))
		return
	}

	// contract type clauses
	contractType, ok := validateInput(r.FormValue("ContractType"), validContractTypes)
	if !ok {
		writeError(c, w, fmt.Errorf("received invalid contract type"))
		return
	}
	docContent, err = insertParas(docContent, clientTemplate.Commercial[contractType], "CommercialTerms")
	if err != nil {
		writeError(c, w, errors.Wrap(err, "error picking up the right contract clauses"))
		return
	}
	params["Output"] = r.FormValue("Output")
	params["OutputPricingDetails"] = r.FormValue("OutputPricingDetails")
	params["MonthlyAmount"] = r.FormValue("MonthlyAmount")

	doc, err := createDoc(params, docContent, 10)
	if err != nil {
		writeError(c, w, errors.Wrap(err, "error creating document"))
		return
	}
	w.Header().Set("Content-Disposition", "attachment; filename=contract.docx")
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	doc.Save(w)
}

func insertParas(destination, toInsert []Para, afterTag string) ([]Para, error) {

	pos := -1
	for j, p := range destination {
		if p.Tag == afterTag {
			pos = j
			break
		}
	}

	if pos == -1 {
		return nil, fmt.Errorf("in insertParas could not find tag: %s", afterTag)
	}

	return append(destination[:pos+1], append(toInsert, destination[pos+1:]...)...), nil
}

func createDoc(params map[string]string, docContent []Para, numberingGroups int) (*document.Document, error) {

	// output the documentfrom here

	doc := document.New()

	doc.Numbering = createNumbering()
	nd := doc.Numbering.Definitions()

	doc.BodySection().SetPageMargins(0.5*measurement.Inch, 0.5*measurement.Inch, 0.5*measurement.Inch, 0.5*measurement.Inch, measurement.Zero, measurement.Zero, measurement.Zero)
	for _, para := range docContent {
		buf := bytes.Buffer{}
		tmpl := template.Must(template.New("test").Parse(para.Text))
		if err := tmpl.Execute(&buf, params); err != nil {
			return nil, errors.Wrapf(err, "error in template parsing at line : %s", para.Text)
		}

		// split the result of the template into lines. We will AddBreak manually
		textLines := strings.Split(strings.Replace(strings.Replace(buf.String(), "\r\n", "\n", -1), "\r", "\n", -1), "\n")

		p := doc.AddParagraph()
		r := p.AddRun()

		for j, text := range textLines {
			r.AddText(text)
			if j != len(textLines)-1 {
				r.AddBreak()
			}
		}

		for _, option := range para.Options {
			switch option {
			case OptCenter:
				p.Properties().SetAlignment(wml.ST_JcCenter)
			case OptNormalSpacing:
				p.Properties().SetSpacing(6*measurement.Point, 6*measurement.Point)
			case OptBeforeOnlySpacing:
				p.Properties().SetSpacing(6*measurement.Point, measurement.Zero)
			case OptAfterOnlySpacing:
				p.Properties().SetSpacing(measurement.Zero, 6*measurement.Point)
			case OptUnderline:
				r.Properties().SetUnderline(wml.ST_UnderlineSingle, color.Black)
			case OptNumberingLevel1:
				p.SetNumberingLevel(0)
			case OptNumberingLevel2:
				p.SetNumberingLevel(1)
			case OptNumberingLevel3:
				p.SetNumberingLevel(2)
			case OptNumberingGroup01:
				p.SetNumberingDefinition(nd[0])
			case OptNumberingGroup02:
				p.SetNumberingDefinition(nd[1])
			case OptNumberingGroup03:
				p.SetNumberingDefinition(nd[2])
			case OptNumberingGroup04:
				p.SetNumberingDefinition(nd[3])
			case OptNumberingGroup05:
				p.SetNumberingDefinition(nd[4])
			case OptNumberingGroup06:
				p.SetNumberingDefinition(nd[5])
			case OptNumberingGroup07:
				p.SetNumberingDefinition(nd[6])
			case OptNumberingGroup08:
				p.SetNumberingDefinition(nd[7])
			case OptNumberingGroup09:
				p.SetNumberingDefinition(nd[8])
			case OptNumberingGroup10:
				p.SetNumberingDefinition(nd[9])
			}
		}

		// default options - font Calibri, size 11
		r.Properties().SetFontFamily("Calibri")
		r.Properties().SetSize(11)

	}

	return doc, nil

}

func createNumbering() document.Numbering {
	numbering := document.NewNumbering()
	for j := 0; j < 5; j++ {
		nd := numbering.AddDefinition()
		for j := 0; j < 8; j++ {
			lvl := nd.AddLevel()
			lvl.SetFormat(wml.ST_NumberFormatDecimal)
			lvl.SetAlignment(wml.ST_JcLeft)
			lvl.SetText(genNumberFormat(j))
			lvl.Properties().SetLeftIndent(measurement.Distance(j) * measurement.Inch * 0.25)

			stNum := wml.NewCT_DecimalNumber()
			stNum.ValAttr = 1
			lvl.X().Start = stNum
		}
	}
	return numbering
}

func genNumberFormat(level int) string {
	ret := ""
	for j := 0; j < level+1; j++ {
		ret = ret + "%" + strconv.Itoa(j+1) + "."
	}
	return ret
}
