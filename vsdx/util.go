package vsdx

import (
	"os"
	"path/filepath"

	"github.com/beevik/etree"
)

// writeFileBytes writes data to a file, creating parent directories as needed.
func writeFileBytes(filename string, data []byte) error {
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filename, data, 0644)
}

// writeXMLBytes serialises a Document with Visio's canonical attribute style
// (single-quote on every attribute). the writer notes §1: Visio's resave path
// always emits attr='value', not attr="value". etree's WriteSettings exposes
// the toggle so we just flip it before each serialisation.
func writeXMLBytes(doc *etree.Document) ([]byte, error) {
	doc.WriteSettings = etree.WriteSettings{AttrSingleQuote: true}
	return doc.WriteToBytes()
}
