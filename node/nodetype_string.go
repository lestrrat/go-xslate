// Code generated by "stringer -type=NodeType node/node.go"; DO NOT EDIT

package node

import "fmt"

const _NodeType_name = "NoopRootTextNumberIntFloatIfElseListForeachWhileWrapperIncludeAssignmentLocalVarFetchFieldFetchArrayElementMethodCallFunCallPrintPrintRawFetchSymbolRangePlusMinusMulDivEqualsNotEqualsLTGTMakeArrayGroupFilterMacroMax"

var _NodeType_index = [...]uint8{0, 4, 8, 12, 18, 21, 26, 28, 32, 36, 43, 48, 55, 62, 72, 80, 90, 107, 117, 124, 129, 137, 148, 153, 157, 162, 165, 168, 174, 183, 185, 187, 196, 201, 207, 212, 215}

func (i NodeType) String() string {
	if i < 0 || i >= NodeType(len(_NodeType_index)-1) {
		return fmt.Sprintf("NodeType(%d)", i)
	}
	return _NodeType_name[_NodeType_index[i]:_NodeType_index[i+1]]
}
