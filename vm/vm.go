/*

Package vm implements the virtual machine used to run the bytecode
generated by http://github.com/lestrrat/go-xslate/compiler 

The virtual machine is an extremely simple one: each opcode in the
bytecode sequence returns the next opcode to execute. The virtual machine
just keeps on calling the opcode until we reach the "end" opcode.

The virtual machine accepts the bytecode, and input variables:

  vm.Run(bytecode, variables)

*/
package vm

import (
  "bytes"
)

// This interface exists solely to avoid importing loader.ByteCodeLoader
// which is a cause for import loop
type byteCodeLoader interface {
  Load(string) (*ByteCode, error)
}

// VM represents the Xslate Virtual Machine
type VM struct {
  st *State
  Loader byteCodeLoader
}

// NewVM creates a new VM
func NewVM() (*VM) {
  return &VM { NewState(), nil }
}

// CurrentOp returns the current Op to be executed
func (vm *VM) CurrentOp() *Op {
  return vm.st.CurrentOp()
}

// Output returns the output accumulated so far
func (vm *VM) Output() ([]byte, error) {
  return vm.st.Output()
}

// OutputString returns the output accumulated so far in string format
func (vm *VM) OutputString() (string, error) {
  return vm.st.OutputString()
}

// Reset reinitializes certain state variables
func (vm *VM) Reset() {
  vm.st.opidx = 0
  vm.st.output = &bytes.Buffer {}
}

// Run executes the given vm.ByteCode using the given variables. For historical
// reasons, it also allows re-executing the previous bytecode instructions
// given to a virtual machine, but this will probably be removed in the future
func (vm *VM) Run(bc *ByteCode, v Vars) {
  vm.Reset()
  st := vm.st
  if bc != nil {
    st.pc = bc
  }
  if v != nil {
    st.vars = v
  }
  st.Loader = vm.Loader
  for op := st.CurrentOp(); op.Type() != TXOPEnd; op = st.CurrentOp() {
    op.Call(st)
  }
}