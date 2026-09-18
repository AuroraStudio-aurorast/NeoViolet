package lyrics

import "github.com/WhatDamon/go-amll-ttml-parser"

// ttmlFormat is the name the TTML parser is registered under (see the
// RegisterParser call in ttml.go). registry.go sets Data.Format to this same
// name, so writing it here keeps a directly-driven adapter consistent with one
// driven through the registry.
const ttmlFormat = "ttml"

// ttmlToData maps an AMLL TTML document onto Neoviolet's Data. It is a pure
// mapping: no IO, no time arithmetic.
//
// Task 1 only fills Path and Format, so the document is not read yet; the
// Lines/Agents/Properties projection - and therefore the parameter's name -
// arrives in task 2.
func ttmlToData(_ *amllttml.Document, sourcePath string) *Data {
	return &Data{
		Path:   sourcePath,
		Format: ttmlFormat,
	}
}
