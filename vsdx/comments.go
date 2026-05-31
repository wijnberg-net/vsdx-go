package vsdx

import (
	"strconv"
	"time"

	"github.com/beevik/etree"
)

// Comment represents a comment/annotation in a Visio document.
type Comment struct {
	ID       int       // Unique comment ID
	AuthorID int       // ID of the comment author
	PageID   int       // Page the comment is on
	ShapeID  int       // Shape the comment is attached to (0 = page-level)
	Date     time.Time // When the comment was created
	EditDate time.Time // When the comment was last edited
	Done     bool      // Whether the comment is marked as resolved
	Text     string    // Comment text content
	xml      *etree.Element
	vis      *VisioFile
}

// Author represents a comment author.
type Author struct {
	ID       int    // Unique author ID
	Name     string // Author name
	Initials string // Author initials
	Color    int    // Author color index
	xml      *etree.Element
}

// Comments returns all comments in the document.
func (v *VisioFile) Comments() []*Comment {
	commentsXML := v.getCommentsXML()
	if commentsXML == nil {
		return nil
	}

	var comments []*Comment
	commentList := commentsXML.FindElement("CommentList")
	if commentList == nil {
		return nil
	}

	for _, elem := range commentList.SelectElements("CommentEntry") {
		comment := parseComment(elem, v)
		if comment != nil {
			comments = append(comments, comment)
		}
	}
	return comments
}

// CommentsForPage returns all comments on a specific page.
func (v *VisioFile) CommentsForPage(pageID int) []*Comment {
	var pageComments []*Comment
	for _, c := range v.Comments() {
		if c.PageID == pageID {
			pageComments = append(pageComments, c)
		}
	}
	return pageComments
}

// CommentsForShape returns all comments on a specific shape.
func (v *VisioFile) CommentsForShape(pageID, shapeID int) []*Comment {
	var shapeComments []*Comment
	for _, c := range v.Comments() {
		if c.PageID == pageID && c.ShapeID == shapeID {
			shapeComments = append(shapeComments, c)
		}
	}
	return shapeComments
}

// Authors returns all comment authors in the document.
func (v *VisioFile) Authors() []*Author {
	commentsXML := v.getCommentsXML()
	if commentsXML == nil {
		return nil
	}

	var authors []*Author
	authorList := commentsXML.FindElement("AuthorList")
	if authorList == nil {
		return nil
	}

	for _, elem := range authorList.SelectElements("AuthorEntry") {
		author := parseAuthor(elem)
		if author != nil {
			authors = append(authors, author)
		}
	}
	return authors
}

// GetAuthor returns the author with the given ID.
func (v *VisioFile) GetAuthor(id int) *Author {
	for _, a := range v.Authors() {
		if a.ID == id {
			return a
		}
	}
	return nil
}

// AddComment adds a new comment to a page.
func (v *VisioFile) AddComment(pageID int, text string, authorName string) *Comment {
	return v.AddShapeComment(pageID, 0, text, authorName)
}

// AddShapeComment adds a new comment to a specific shape.
func (v *VisioFile) AddShapeComment(pageID, shapeID int, text string, authorName string) *Comment {
	commentsXML := v.getOrCreateCommentsXML()

	// Find or create author
	author := v.findOrCreateAuthor(commentsXML, authorName)

	// Create comment
	commentList := commentsXML.FindElement("CommentList")
	if commentList == nil {
		commentList = commentsXML.CreateElement("CommentList")
	}

	// Get next comment ID
	commentID := 1
	for _, elem := range commentList.SelectElements("CommentEntry") {
		if id, _ := strconv.Atoi(elem.SelectAttrValue("CommentID", "0")); id >= commentID {
			commentID = id + 1
		}
	}

	now := time.Now()
	elem := commentList.CreateElement("CommentEntry")
	elem.CreateAttr("AuthorID", strconv.Itoa(author.ID))
	elem.CreateAttr("PageID", strconv.Itoa(pageID))
	if shapeID > 0 {
		elem.CreateAttr("ShapeID", strconv.Itoa(shapeID))
	}
	elem.CreateAttr("Date", now.Format(time.RFC3339))
	elem.CreateAttr("CommentID", strconv.Itoa(commentID))
	elem.SetText(text)

	v.markCommentsModified()

	return &Comment{
		ID:       commentID,
		AuthorID: author.ID,
		PageID:   pageID,
		ShapeID:  shapeID,
		Date:     now,
		EditDate: now,
		Text:     text,
		xml:      elem,
		vis:      v,
	}
}

// Delete removes the comment from the document.
func (c *Comment) Delete() {
	if c.xml != nil && c.xml.Parent() != nil {
		c.xml.Parent().RemoveChild(c.xml)
		c.vis.markCommentsModified()
	}
}

// SetText updates the comment text.
func (c *Comment) SetText(text string) {
	c.Text = text
	if c.xml != nil {
		c.xml.SetText(text)
		c.EditDate = time.Now()
		c.xml.CreateAttr("EditDate", c.EditDate.Format(time.RFC3339))
		c.vis.markCommentsModified()
	}
}

// SetDone marks the comment as resolved or unresolved.
func (c *Comment) SetDone(done bool) {
	c.Done = done
	if c.xml != nil {
		if done {
			c.xml.CreateAttr("Done", "1")
		} else {
			c.xml.CreateAttr("Done", "0")
		}
		c.vis.markCommentsModified()
	}
}

// Author returns the author of this comment.
func (c *Comment) Author() *Author {
	return c.vis.GetAuthor(c.AuthorID)
}

// parseComment parses a CommentEntry element into a Comment struct.
func parseComment(elem *etree.Element, vis *VisioFile) *Comment {
	comment := &Comment{
		xml: elem,
		vis: vis,
	}

	comment.ID, _ = strconv.Atoi(elem.SelectAttrValue("CommentID", "0"))
	comment.AuthorID, _ = strconv.Atoi(elem.SelectAttrValue("AuthorID", "0"))
	comment.PageID, _ = strconv.Atoi(elem.SelectAttrValue("PageID", "0"))
	comment.ShapeID, _ = strconv.Atoi(elem.SelectAttrValue("ShapeID", "0"))
	comment.Done = elem.SelectAttrValue("Done", "0") == "1"
	comment.Text = elem.Text()

	if dateStr := elem.SelectAttrValue("Date", ""); dateStr != "" {
		comment.Date, _ = time.Parse(time.RFC3339, dateStr)
	}
	if editStr := elem.SelectAttrValue("EditDate", ""); editStr != "" {
		comment.EditDate, _ = time.Parse(time.RFC3339, editStr)
	}

	return comment
}

// parseAuthor parses an AuthorEntry element into an Author struct.
func parseAuthor(elem *etree.Element) *Author {
	author := &Author{
		xml: elem,
	}

	author.ID, _ = strconv.Atoi(elem.SelectAttrValue("ID", "0"))
	author.Name = elem.SelectAttrValue("Name", "")
	author.Initials = elem.SelectAttrValue("Initials", "")
	author.Color, _ = strconv.Atoi(elem.SelectAttrValue("ResolutionID", "0"))

	return author
}

// getCommentsXML returns the parsed comments.xml document.
func (v *VisioFile) getCommentsXML() *etree.Element {
	data, ok := v.ZipFileContents["visio/comments.xml"]
	if !ok {
		return nil
	}

	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(data); err != nil {
		return nil
	}

	return doc.Root()
}

// getOrCreateCommentsXML returns or creates the comments.xml document.
func (v *VisioFile) getOrCreateCommentsXML() *etree.Element {
	root := v.getCommentsXML()
	if root != nil {
		return root
	}

	// Create new comments document
	doc := etree.NewDocument()
	doc.CreateProcInst("xml", `version="1.0" encoding="UTF-8" standalone="yes"`)
	root = doc.CreateElement("Comments")
	root.CreateAttr("xmlns", MainNS)
	root.CreateElement("AuthorList")
	root.CreateElement("CommentList")

	// Store in zip contents
	data, _ := writeXMLBytes(doc)
	v.ZipFileContents["visio/comments.xml"] = data

	// Add to content types
	v.addContentType("/visio/comments.xml", "application/vnd.ms-visio.comments+xml")

	// Add relationship
	v.addRelationship("visio/comments.xml", "http://schemas.microsoft.com/visio/2010/relationships/comments")

	return root
}

// findOrCreateAuthor finds an existing author or creates a new one.
func (v *VisioFile) findOrCreateAuthor(commentsXML *etree.Element, name string) *Author {
	authorList := commentsXML.FindElement("AuthorList")
	if authorList == nil {
		authorList = commentsXML.CreateElement("AuthorList")
	}

	// Look for existing author
	for _, elem := range authorList.SelectElements("AuthorEntry") {
		if elem.SelectAttrValue("Name", "") == name {
			return parseAuthor(elem)
		}
	}

	// Create new author
	authorID := 0
	for _, elem := range authorList.SelectElements("AuthorEntry") {
		if id, _ := strconv.Atoi(elem.SelectAttrValue("ID", "0")); id >= authorID {
			authorID = id + 1
		}
	}

	elem := authorList.CreateElement("AuthorEntry")
	elem.CreateAttr("ID", strconv.Itoa(authorID))
	elem.CreateAttr("Name", name)
	elem.CreateAttr("Initials", getInitials(name))

	return &Author{
		ID:       authorID,
		Name:     name,
		Initials: getInitials(name),
		xml:      elem,
	}
}

// getInitials extracts initials from a name.
func getInitials(name string) string {
	if name == "" {
		return ""
	}
	initials := ""
	prevSpace := true
	for _, r := range name {
		if r == ' ' {
			prevSpace = true
		} else if prevSpace {
			initials += string(r)
			prevSpace = false
		}
	}
	return initials
}

// markCommentsModified marks the comments.xml as modified.
func (v *VisioFile) markCommentsModified() {
	// Re-serialize and store
	root := v.getCommentsXML()
	if root != nil {
		doc := etree.NewDocument()
		doc.CreateProcInst("xml", `version="1.0" encoding="UTF-8" standalone="yes"`)
		doc.SetRoot(root.Copy())
		if data, err := writeXMLBytes(doc); err == nil {
			v.ZipFileContents["visio/comments.xml"] = data
		}
	}
}

// addContentType adds a content type to [Content_Types].xml.
func (v *VisioFile) addContentType(partName, contentType string) {
	data, ok := v.ZipFileContents["[Content_Types].xml"]
	if !ok {
		return
	}

	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(data); err != nil {
		return
	}

	root := doc.Root()
	if root == nil {
		return
	}

	// Check if already exists
	for _, override := range root.SelectElements("Override") {
		if override.SelectAttrValue("PartName", "") == partName {
			return
		}
	}

	// Add override
	override := root.CreateElement("Override")
	override.CreateAttr("PartName", partName)
	override.CreateAttr("ContentType", contentType)

	data, _ = writeXMLBytes(doc)
	v.ZipFileContents["[Content_Types].xml"] = data
}

// addRelationship adds a relationship to document.xml.rels.
func (v *VisioFile) addRelationship(target, relType string) {
	relPath := "visio/_rels/document.xml.rels"
	data, ok := v.ZipFileContents[relPath]
	if !ok {
		return
	}

	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(data); err != nil {
		return
	}

	root := doc.Root()
	if root == nil {
		return
	}

	// Check if already exists
	for _, rel := range root.SelectElements("Relationship") {
		if rel.SelectAttrValue("Target", "") == target {
			return
		}
	}

	// Get next relationship ID
	maxID := 0
	for _, rel := range root.SelectElements("Relationship") {
		id := rel.SelectAttrValue("Id", "")
		if len(id) > 3 {
			if n, err := strconv.Atoi(id[3:]); err == nil && n > maxID {
				maxID = n
			}
		}
	}

	// Add relationship
	rel := root.CreateElement("Relationship")
	rel.CreateAttr("Id", "rId"+strconv.Itoa(maxID+1))
	rel.CreateAttr("Type", relType)
	rel.CreateAttr("Target", target)

	data, _ = writeXMLBytes(doc)
	v.ZipFileContents[relPath] = data
}
