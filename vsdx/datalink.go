package vsdx

import (
	"strconv"

	"github.com/beevik/etree"
)

// DataConnection represents an external data connection.
type DataConnection struct {
	ID             int
	FileName       string
	ConnectionName string
	AlwaysUseConnectionFile bool
	Command        string
	Timeout        int
	xml            *etree.Element
}

// DataRecordSet represents a set of data records linked to shapes.
type DataRecordSet struct {
	ID               int
	ConnectionID     int
	Name             string
	Command          string
	RefreshInterval  int
	RefreshOnOpen    bool
	RefreshOverwrite bool
	Columns          []DataColumn
	Records          []DataRecord
	xml              *etree.Element
}

// DataColumn represents a column in a data recordset.
type DataColumn struct {
	ID       int
	Name     string
	Label    string
	DataType string
	LangID   int
	Currency int
	xml      *etree.Element
}

// DataRecord represents a row of data in a recordset.
type DataRecord struct {
	RowID  int
	Values map[string]string
	xml    *etree.Element
}

// DataConnections returns all data connections in the document.
func (v *VisioFile) DataConnections() []*DataConnection {
	data, ok := v.ZipFileContents["visio/data/connections.xml"]
	if !ok {
		return nil
	}

	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(data); err != nil {
		return nil
	}

	root := doc.Root()
	if root == nil {
		return nil
	}

	var connections []*DataConnection
	for _, elem := range root.SelectElements("DataConnection") {
		conn := parseDataConnection(elem)
		if conn != nil {
			connections = append(connections, conn)
		}
	}

	return connections
}

// GetDataConnection returns the data connection with the given ID.
func (v *VisioFile) GetDataConnection(id int) *DataConnection {
	for _, conn := range v.DataConnections() {
		if conn.ID == id {
			return conn
		}
	}
	return nil
}

// DataRecordSets returns all data recordsets in the document.
func (v *VisioFile) DataRecordSets() []*DataRecordSet {
	data, ok := v.ZipFileContents["visio/data/recordsets.xml"]
	if !ok {
		return nil
	}

	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(data); err != nil {
		return nil
	}

	root := doc.Root()
	if root == nil {
		return nil
	}

	var recordsets []*DataRecordSet
	for _, elem := range root.SelectElements("DataRecordSet") {
		rs := parseDataRecordSet(elem)
		if rs != nil {
			recordsets = append(recordsets, rs)
		}
	}

	return recordsets
}

// GetDataRecordSet returns the data recordset with the given ID.
func (v *VisioFile) GetDataRecordSet(id int) *DataRecordSet {
	for _, rs := range v.DataRecordSets() {
		if rs.ID == id {
			return rs
		}
	}
	return nil
}

// LinkedDataRecordSet returns the ID of the data recordset this shape is linked to.
func (s *Shape) LinkedDataRecordSet() int {
	if val := s.CellValue("DataLinked"); val != "" {
		id, _ := strconv.Atoi(val)
		return id
	}
	return 0
}

// LinkedDataRow returns the row ID within the linked data recordset.
func (s *Shape) LinkedDataRow() int {
	// The link is typically stored in a Data1, Data2, or Data3 cell
	// or via shape data properties with specific naming
	if val := s.CellValue("Data1"); val != "" {
		id, _ := strconv.Atoi(val)
		return id
	}
	return 0
}

// LinkToData links this shape to a data recordset row.
func (s *Shape) LinkToData(recordSetID, rowID int) {
	s.SetCellValue("DataLinked", strconv.Itoa(recordSetID))
	s.SetCellValue("Data1", strconv.Itoa(rowID))
}

// UnlinkData removes the data link from this shape.
func (s *Shape) UnlinkData() {
	s.SetCellValue("DataLinked", "0")
	s.SetCellValue("Data1", "")
}

// parseDataConnection parses a DataConnection element.
func parseDataConnection(elem *etree.Element) *DataConnection {
	conn := &DataConnection{xml: elem}

	conn.ID, _ = strconv.Atoi(elem.SelectAttrValue("ID", "0"))
	conn.FileName = elem.SelectAttrValue("FileName", "")
	conn.ConnectionName = elem.SelectAttrValue("ConnectionString", "")
	conn.AlwaysUseConnectionFile = elem.SelectAttrValue("AlwaysUseConnectionFile", "0") == "1"
	conn.Command = elem.SelectAttrValue("Command", "")
	conn.Timeout, _ = strconv.Atoi(elem.SelectAttrValue("Timeout", "0"))

	return conn
}

// parseDataRecordSet parses a DataRecordSet element.
func parseDataRecordSet(elem *etree.Element) *DataRecordSet {
	rs := &DataRecordSet{xml: elem}

	rs.ID, _ = strconv.Atoi(elem.SelectAttrValue("ID", "0"))
	rs.ConnectionID, _ = strconv.Atoi(elem.SelectAttrValue("ConnectionID", "0"))
	rs.Name = elem.SelectAttrValue("Name", "")
	rs.Command = elem.SelectAttrValue("Command", "")
	rs.RefreshInterval, _ = strconv.Atoi(elem.SelectAttrValue("RefreshInterval", "0"))
	rs.RefreshOnOpen = elem.SelectAttrValue("RefreshNoReconciliationUI", "0") == "1"
	rs.RefreshOverwrite = elem.SelectAttrValue("RefreshOverwriteAll", "0") == "1"

	// Parse columns
	for _, colElem := range elem.SelectElements("DataColumn") {
		col := parseDataColumn(colElem)
		if col != nil {
			rs.Columns = append(rs.Columns, *col)
		}
	}

	// Parse records
	for _, recElem := range elem.SelectElements("DataRecords/DataRecord") {
		rec := parseDataRecord(recElem, rs.Columns)
		if rec != nil {
			rs.Records = append(rs.Records, *rec)
		}
	}

	return rs
}

// parseDataColumn parses a DataColumn element.
func parseDataColumn(elem *etree.Element) *DataColumn {
	col := &DataColumn{xml: elem}

	col.ID, _ = strconv.Atoi(elem.SelectAttrValue("ColumnNameID", "0"))
	col.Name = elem.SelectAttrValue("Name", "")
	col.Label = elem.SelectAttrValue("Label", "")
	col.DataType = elem.SelectAttrValue("DataType", "")
	col.LangID, _ = strconv.Atoi(elem.SelectAttrValue("LangID", "0"))
	col.Currency, _ = strconv.Atoi(elem.SelectAttrValue("Currency", "0"))

	return col
}

// parseDataRecord parses a DataRecord element.
func parseDataRecord(elem *etree.Element, columns []DataColumn) *DataRecord {
	rec := &DataRecord{
		xml:    elem,
		Values: make(map[string]string),
	}

	rec.RowID, _ = strconv.Atoi(elem.SelectAttrValue("RowID", "0"))

	// Parse cell values
	for i, cell := range elem.SelectElements("Cell") {
		value := cell.SelectAttrValue("V", "")
		if i < len(columns) {
			rec.Values[columns[i].Name] = value
		}
	}

	return rec
}

// CreateDataRecordSet creates a new data recordset for manual data entry.
func (v *VisioFile) CreateDataRecordSet(name string, columns []string) *DataRecordSet {
	data, ok := v.ZipFileContents["visio/data/recordsets.xml"]

	var doc *etree.Document
	var root *etree.Element

	if ok {
		doc = etree.NewDocument()
		if err := doc.ReadFromBytes(data); err != nil {
			return nil
		}
		root = doc.Root()
	} else {
		doc = etree.NewDocument()
		doc.CreateProcInst("xml", `version="1.0" encoding="UTF-8" standalone="yes"`)
		root = doc.CreateElement("DataRecordSets")
		root.CreateAttr("xmlns", MainNS)

		// Update content types
		v.addContentType("/visio/data/recordsets.xml", "application/vnd.ms-visio.data.recordsets+xml")
	}

	// Get next ID
	maxID := 0
	for _, elem := range root.SelectElements("DataRecordSet") {
		if id, _ := strconv.Atoi(elem.SelectAttrValue("ID", "0")); id > maxID {
			maxID = id
		}
	}

	// Create recordset element
	rsElem := root.CreateElement("DataRecordSet")
	rsElem.CreateAttr("ID", strconv.Itoa(maxID+1))
	rsElem.CreateAttr("Name", name)

	// Add columns
	var cols []DataColumn
	for i, colName := range columns {
		colElem := rsElem.CreateElement("DataColumn")
		colElem.CreateAttr("ColumnNameID", strconv.Itoa(i))
		colElem.CreateAttr("Name", colName)
		colElem.CreateAttr("Label", colName)
		colElem.CreateAttr("DataType", "0") // String

		cols = append(cols, DataColumn{
			ID:    i,
			Name:  colName,
			Label: colName,
		})
	}

	// Create DataRecords container
	rsElem.CreateElement("DataRecords")

	// Save
	data, _ = writeXMLBytes(doc)
	v.ZipFileContents["visio/data/recordsets.xml"] = data

	return &DataRecordSet{
		ID:      maxID + 1,
		Name:    name,
		Columns: cols,
		xml:     rsElem,
	}
}

// AddRecord adds a new record to a data recordset.
func (rs *DataRecordSet) AddRecord(values map[string]string) *DataRecord {
	if rs.xml == nil {
		return nil
	}

	records := rs.xml.FindElement("DataRecords")
	if records == nil {
		records = rs.xml.CreateElement("DataRecords")
	}

	// Get next row ID
	maxRowID := 0
	for _, rec := range records.SelectElements("DataRecord") {
		if id, _ := strconv.Atoi(rec.SelectAttrValue("RowID", "0")); id > maxRowID {
			maxRowID = id
		}
	}

	// Create record
	recElem := records.CreateElement("DataRecord")
	recElem.CreateAttr("RowID", strconv.Itoa(maxRowID+1))

	// Add cells for each column
	for _, col := range rs.Columns {
		cell := recElem.CreateElement("Cell")
		cell.CreateAttr("N", col.Name)
		if val, ok := values[col.Name]; ok {
			cell.CreateAttr("V", val)
		}
	}

	record := &DataRecord{
		RowID:  maxRowID + 1,
		Values: values,
		xml:    recElem,
	}
	rs.Records = append(rs.Records, *record)

	return record
}

// GetRecord returns the record with the specified row ID.
func (rs *DataRecordSet) GetRecord(rowID int) *DataRecord {
	for i := range rs.Records {
		if rs.Records[i].RowID == rowID {
			return &rs.Records[i]
		}
	}
	return nil
}
