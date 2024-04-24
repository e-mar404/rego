/*
# Google Workspace - Sheets

This package initializes all the methods for functions which interact with the Google Sheets API:
https://developers.google.com/sheets/api/reference/rest

:Copyright: (c) 2023 by Gemini Space Station, LLC, see AUTHORS for more info
:License: See the LICENSE file for details
:Author: Anthony Dardano <anthony.dardano@gemini.com>
*/

// pkg/google/sheets.go
package google

import (
	"fmt"
	"reflect"
	"time"

	ss "github.com/gemini-oss/rego/pkg/common/starstruct"
)

var (
	SheetsBaseURL          = "https://sheets.googleapis.com/v4"
	Sheets                 = fmt.Sprintf("%s/spreadsheets", SheetsBaseURL)             // https://developers.google.com/sheets/api/reference/rest/v4/spreadsheets
	SheetByID              = fmt.Sprintf("%s/%s", Sheets, "%s")                        // https://developers.google.com/sheets/api/reference/rest/v4/spreadsheets/get
	SheetValuesRange       = fmt.Sprintf("%s/%s/values/%s", Sheets, "%s", "%s")        // https://developers.google.com/sheets/api/reference/rest/v4/spreadsheets.values/get
	SheetValuesBatchGet    = fmt.Sprintf("%s/%s/values:batchGet", Sheets, "%s")        // https://developers.google.com/sheets/api/reference/rest/v4/spreadsheets.values/batchGet
	SheetValuesBatchUpdate = fmt.Sprintf("%s/%s/values:batchUpdate", Sheets, "%s")     // https://developers.google.com/sheets/api/reference/rest/v4/spreadsheets.values/batchUpdate
	SheetValuesAppend      = fmt.Sprintf("%s/%s/values/%s:append", Sheets, "%s", "%s") // https://developers.google.com/sheets/api/reference/rest/v4/spreadsheets.values/append
)

// SheetsClient for chaining methods
type SheetsClient struct {
	*Client
}

// Entry point for sheets-related operations
func (c *Client) Sheets() *SheetsClient {
	return &SheetsClient{
		Client: c,
	}
}

/*
 * Query Parameters for Sheet Values
 */
type SheetValueQuery struct {
	Ranges                       []string `url:"ranges,omitempty"`                       // The ranges to retrieve from the spreadsheet.
	IncludeGridData              bool     `url:"includeGridData,omitempty"`              // True if grid data should be returned. This parameter is ignored if a field mask was set in the request.
	MajorDimension               string   `url:"majorDimension,omitempty"`               // https://developers.google.com/sheets/api/reference/rest/v4/Dimension
	ValueRenderOption            string   `url:"valueRenderOption,omitempty"`            // https://developers.google.com/sheets/api/reference/rest/v4/ValueRenderOption
	DateTimeRenderOption         string   `url:"dateTimeRenderOption,omitempty"`         // https://developers.google.com/sheets/api/reference/rest/v4/DateTimeRenderOption
	ValueInputOption             string   `url:"valueInputOption,omitempty"`             // How the input data should be interpreted. Accepted values are: RAW or USER_ENTERED. The default is USER_ENTERED.
	IncludeValuesInResponse      bool     `url:"includeValuesInResponse,omitempty"`      // Determines if the update response should include the values of the cells that were updated. By default, responses do not include the updated values. If the range to write was larger than the range actually written, the response includes all values in the requested range (excluding trailing empty rows and columns).
	ResponseValueRenderOption    string   `url:"responseValueRenderOption,omitempty"`    // Determines how values in the response should be rendered. The default render option is FORMATTED_VALUE.
	ResponseDateTimeRenderOption string   `url:"responseDateTimeRenderOption,omitempty"` // Determines how dates, times, and durations in the response should be rendered. This is ignored if responseValueRenderOption is FORMATTED_VALUE. The default dateTime render option is SERIAL_NUMBER.
}

/*
 * # Set Sheet Value Defaults
 * - Sets default values for ValueRange if they are not defined
 */
func (c *SheetsClient) VerifySheetValueRange(vr *ValueRange) error {
	if vr.Range == "" {
		vr.Range = "A:Z"
	}
	if vr.MajorDimension == "" {
		vr.MajorDimension = "ROWS"
	}
	if vr.Values == nil {
		return fmt.Errorf("ValueRange.Values cannot be empty")
	}
	return nil
}

/*
 * Generate Google Sheets ValueRange from a slice of any structs
 */
func (c *SheetsClient) GenerateValueRange(data []interface{}, sheetName string, headers *[]string) *ValueRange {
	vr := &ValueRange{
		MajorDimension: "ROWS",
	}

	if sheetName != "" {
		vr.Range = fmt.Sprintf("%s!A:ZZ", sheetName)
	} else {
		vr.Range = "A:ZZ"
	}

	vr.Values = append(vr.Values, *headers)
	for _, d := range data {
		orderedData, err := ss.FlattenStructFields(d, headers)
		if err != nil {
			continue // Owners field is empty -- skip
		}
		row := make([]string, 0, len(*headers))
		for i, value := range orderedData {
			// If the value matches the header, append it to the row
			if value[0] == (*headers)[i] {
				row = append(row, value[1])
			}
		}
		vr.Values = append(vr.Values, row)
	}
	vr.Values[0] = *headers

	return vr
}

/*
 * # Spreadsheet: Create
 * - Creates a new spreadsheet, with basic properties.
 *   - https://developers.google.com/sheets/api/reference/rest/v4/spreadsheets/create
 */
func (c *SheetsClient) CreateSpreadsheet(s *Spreadsheet) (*Spreadsheet, error) {
	url := Sheets

	spreadsheet, err := do[Spreadsheet](c.Client, "POST", url, nil, s)
	if err != nil {
		return nil, err
	}

	return &spreadsheet, nil
}

/*
 * # Spreadsheet Values: Update
 * Sets/Replaces values in a range of a spreadsheet. The caller must specify the spreadsheet ID, range, and a valueInputOption
 * spreadsheets/{spreadsheetId}/values/{range}
 * https://developers.google.com/sheets/api/reference/rest/v4/spreadsheets.values/update
 */
func (c *SheetsClient) UpdateSpreadsheet(spreadsheetID string, vr *ValueRange) error {

	q := SheetValueQuery{
		ValueInputOption: "RAW",
	}

	// Check Value paramters
	err := c.VerifySheetValueRange(vr)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/%s/values/%s", Sheets, spreadsheetID, vr.Range)

	_, err = do[any](c.Client, "PUT", url, q, &vr)
	if err != nil {
		return err
	}

	return nil
}

/*
 * # Spreadsheet Values: Append
 * - Appends values within the range of a spreadsheet. The caller must specify the spreadsheet ID, range, and a valueInputOption
 *   - https://sheets.googleapis.com/v4/spreadsheets/{spreadsheetId}/values/{range}
 *   - https://developers.google.com/sheets/api/reference/rest/v4/spreadsheets.values/update
 */
func (c *SheetsClient) AppendSpreadsheet(spreadsheetID string, vr *ValueRange) error {

	q := SheetValueQuery{
		ValueInputOption: "RAW",
	}

	// Check Value paramters
	err := c.VerifySheetValueRange(vr)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/%s/values/%s:append", Sheets, spreadsheetID, vr.Range)

	_, err = do[any](c.Client, "POST", url, q, &vr)
	if err != nil {
		return err
	}

	return nil
}

/*
 * # Format Header and AutoSize
 * - Sets the header row to bold and green, and auto-sizes all columns
 */
func (c *SheetsClient) FormatHeaderAndAutoSize(spreadsheetID string, sheet *Sheet, rows, columns int) error {
	url := fmt.Sprintf("%s/%s:batchUpdate", Sheets, spreadsheetID)

	format := &SheetBatchRequest{}

	// Set the header row to bold and green
	format.Requests = append(format.Requests, &SheetRequest{
		RepeatCell: &RepeatCellRequest{
			Range: &GridRange{
				SheetID:          sheet.Properties.SheetID,
				StartRowIndex:    0,
				EndRowIndex:      1,
				StartColumnIndex: 0,
				EndColumnIndex:   columns,
			},
			Cell: &CellData{
				UserEnteredFormat: &CellFormat{
					BackgroundColor: &Color{
						Alpha: 1.0,
						Red:   (182.0 / 255.0),
						Green: (215.0 / 255.0),
						Blue:  (168.0 / 255.0),
					},
					TextFormat: &TextFormat{
						FontSize: 10,
						Bold:     true,
					},
				},
			},
			Fields: "userEnteredFormat(backgroundColor,textFormat)",
		},
	})

	// Add a filter view for the header row
	format.Requests = append(format.Requests, &SheetRequest{
		SetBasicFilter: &SetBasicFilterRequest{
			Filter: &BasicFilter{
				Range: &GridRange{
					SheetID:          sheet.Properties.SheetID,
					StartRowIndex:    0,
					EndRowIndex:      rows,
					StartColumnIndex: 0,
					EndColumnIndex:   columns,
				},
			},
		},
	})

	// Auto resize all columns
	format.Requests = append(format.Requests, &SheetRequest{
		AutoResizeDimensions: &AutoResizeDimensionsRequest{
			Dimensions: &DimensionRange{
				SheetID:    sheet.Properties.SheetID,
				Dimension:  "COLUMNS",
				StartIndex: 0,
				EndIndex:   columns,
			},
		},
	})

	// Execute the batchUpdate request
	_, err := do[any](c.Client, "POST", url, nil, format)
	if err != nil {
		return err
	}

	return nil
}

/*
 * # Save to Sheet
 * - Saves a variety of data types to a Google Sheet (array, map, slice, struct)
 */
func (c *SheetsClient) SaveToSheet(data interface{}, sheetID, sheetName string, headers *[]string) error {
	// Dereference all pointers first to simplify further processing
	val, err := ss.DerefPointers(reflect.ValueOf(data))
	if err != nil {
		return err
	}

	// Handle sheet creation if ID isn't provided
	sheet := &Spreadsheet{}
	if sheetID == "" {
		c.Log.Println("Creating new sheet as no sheet ID was provided.")
		newSpreadsheet := &Spreadsheet{
			Properties: &SpreadsheetProperties{
				Title: fmt.Sprintf("{Rego} New Spreadsheet %s", time.Now().Format("2006-01-02 15:04:05")),
			},
			Sheets: []Sheet{
				{
					Properties: &SheetProperties{
						Title: sheetName,
					},
				},
			},
		}
		sheet, err = c.CreateSpreadsheet(newSpreadsheet)
		if err != nil {
			return err
		}
		sheetID = sheet.SpreadsheetID
	} else {
		sheet, err = c.GetSpreadsheet(sheetID)
		if err != nil {
			return err
		}
	}

	if sheetName == "" {
		sheetName = "Sheet1"
	}

	// Generate headers if not provided
	if headers == nil {
		headers, err = ss.GenerateFieldNames("", val)
		if err != nil {
			return err
		}
	}

	vr, err := c.prepareAndGenerateValueRange(val, sheetName, headers)
	if err != nil {
		return err
	}

	c.Log.Println("Updating spreadsheet data.")
	if err := c.UpdateSpreadsheet(sheetID, vr); err != nil {
		return err
	}

	c.Log.Println("Auto-formatting the spreadsheet.")
	rows := len(vr.Values)
	columns := len(vr.Values[0])
	for _, sheet := range sheet.Sheets {
		if sheet.Properties.Title == sheetName {
			c.FormatHeaderAndAutoSize(sheetID, &sheet, rows, columns)
		}
	}

	return nil
}

func (c *SheetsClient) prepareAndGenerateValueRange(val reflect.Value, sheetName string, headers *[]string) (*ValueRange, error) {
	var sheetData []interface{}

	switch val.Kind() {
	case reflect.Map:
		sheetData = make([]interface{}, 0, val.Len())
		for _, key := range val.MapKeys() {
			sheetData = append(sheetData, val.MapIndex(key).Interface())
		}
	case reflect.Slice, reflect.Array:
		sheetData = make([]interface{}, val.Len())
		for i := 0; i < val.Len(); i++ {
			sheetData[i] = val.Index(i).Interface()
		}
	case reflect.Struct:
		sheetData = []interface{}{val.Interface()}
	default:
		return nil, fmt.Errorf("unsupported data type: %s", val.Kind())
	}

	return c.GenerateValueRange(sheetData, sheetName, headers), nil
}

/*
 * # Spreadsheet: Get
 * Get a spreadsheet and its properties by ID
 * spreadsheets/{spreadsheetId}
 * https://developers.google.com/sheets/api/reference/rest/v4/spreadsheets/get
 */
func (c *SheetsClient) GetSpreadsheet(sheetID string) (*Spreadsheet, error) {
	url := fmt.Sprintf(SheetByID, sheetID)

	q := SheetValueQuery{
		IncludeGridData: false,
	}

	spreadsheet, err := do[Spreadsheet](c.Client, "GET", url, q, nil)
	if err != nil {
		return nil, err
	}

	return &spreadsheet, nil
}

/*
 * # Spreadsheet: Read
 * Reads values from a spreadsheet
 * spreadsheets/{spreadsheetId}/values/{range}
 * https://developers.google.com/sheets/api/reference/rest/v4/spreadsheets.values/get
 */
func (c *SheetsClient) ReadSpreadsheetValues(sheetID, rangeNotation string) (*ValueRange, error) {

	if rangeNotation == "" {
		rangeNotation = "Sheet1!A:ZZ"
	}

	q := SheetValueQuery{
		MajorDimension:    "ROWS",
		ValueRenderOption: "UNFORMATTED_VALUE",
	}

	url := fmt.Sprintf("%s/%s/values/%s", Sheets, sheetID, rangeNotation)

	vr, err := do[ValueRange](c.Client, "GET", url, q, nil)
	if err != nil {
		return nil, err
	}

	return &vr, nil
}
