package potatolang

import (
	"fmt"
)

// Env is the environment for a closure in potatolang to run within.
// Env.stack contains arguments used to execute the closure,
// then the local variables will sequentially take the following spaces.
// Env.A stores the result of executing a closure, or a builtin operator;
// Env.R0 ~ Env.R3 store the arguments to call builtin operators (+, -, *, ...).
type Env struct {
	parent *Env
	trace  []stacktrace
	stack  []Value

	A, R0, R1, R2, R3 Value
}

// NewEnv creates the Env for closure to run within
// parent can be nil, which means this is a top Env
func NewEnv(parent *Env) *Env {
	const initCapacity = 16
	return &Env{
		parent: parent,
		stack:  make([]Value, 0, initCapacity),
		A:      Value{},
	}
}

func (env *Env) grow(newSize int) {
	if newSize > cap(env.stack) {
		old := env.stack
		env.stack = make([]Value, newSize, newSize*3/2)
		copy(env.stack, old)
	}
	env.stack = env.stack[:newSize]
}

// SGet gets a value from the current stack
func (env *Env) SGet(index int) Value {
	if index >= len(env.stack) {
		return Value{}
	}
	return env.stack[index]
}

// SSet sets a value in the current stack
func (env *Env) SSet(index int, value Value) {
	if index >= len(env.stack) {
		env.grow(index + 1)
	}
	env.stack[index] = value
}

// SClear clears the current stack
func (env *Env) SClear() {
	env.stack = env.stack[:0]
	env.A = Value{}
}

// SInsert inserts another stack into the current stack
func (env *Env) SInsert(index int, data []Value) {
	if index <= len(env.stack) {
		ln := len(env.stack)
		env.grow(ln + len(data))
		copy(env.stack[len(env.stack)-(ln-index):], env.stack[index:])
	} else {
		env.grow(index + len(data))
	}
	copy(env.stack[index:], data)
}

// SPush pushes a value into the current stack
func (env *Env) SPush(v Value) {
	// e.stack.Add(v)
	ln := len(env.stack)
	env.grow(ln + 1)
	env.stack[ln] = v
}

func (env *Env) SSize() int {
	return len(env.stack)
}

func (e *Env) Parent() *Env {
	return e.parent
}

func (e *Env) SetParent(parent *Env) {
	e.parent = parent
}

func (env *Env) Get(yx uint32) Value {
	if yx == regA {
		return env.A
	}
	y := yx >> 16
REPEAT:
	if y > 0 && env != nil {
		y, env = y-1, env.parent
		goto REPEAT
	}
	index := int(uint16(yx))
	if index >= len(env.stack) {
		return Value{}
	}
	return env.stack[index]
}

func (env *Env) Set(yx uint32, v Value) {
	if yx == regA {
		env.A = v
	} else {
		y := yx >> 16
	REPEAT:
		if y > 0 && env != nil {
			y, env = y-1, env.parent
			goto REPEAT
		}
		index := int(uint16(yx))
		if index >= len(env.stack) {
			env.grow(index + 1)
		}
		env.stack[index] = v
	}
}

// Stack returns the current stack
func (env *Env) Stack() []Value {
	return env.stack
}

// Closure is the closure struct used in potatolang
type Closure struct {
	code        []uint64
	pos         []uint64
	consts      []Value
	env         *Env
	caller      Value
	preArgs     []Value
	native      func(env *Env) Value
	argsCount   byte
	noenvescape bool
	receiver    bool
	ye          byte
	lastp       uint32
	lastenv     *Env
}

// NewClosure creates a new closure
func NewClosure(code []uint64, consts []Value, env *Env, argsCount byte, y, e, r, noenvescape bool) *Closure {
	cls := &Closure{
		code:        code,
		consts:      consts,
		env:         env,
		argsCount:   argsCount,
		noenvescape: noenvescape,
		receiver:    r,
	}
	if y {
		cls.ye = 0x01
	}
	if e {
		cls.ye |= 0x10
	}
	return cls
}

// NewNativeValue creates a native function in potatolang
func NewNativeValue(argsCount int, f func(env *Env) Value) Value {
	return NewClosureValue(&Closure{
		argsCount:   byte(argsCount),
		native:      f,
		noenvescape: true,
	})
}

func (c *Closure) Errorable() bool {
	return (c.ye >> 4) > 0
}

func (c *Closure) Yieldable() bool {
	return (c.ye & 0x01) > 0
}

func (c *Closure) setYieldable(b bool) {
	if b {
		c.ye |= 0x01
	} else {
		c.ye &= 0x10
	}
}

func (c *Closure) setErrorable(b bool) {
	if b {
		c.ye |= 0x10
	} else {
		c.ye &= 0x01
	}
}

func (c *Closure) AppendPreArgs(preArgs []Value) {
	if c.preArgs == nil {
		c.preArgs = make([]Value, 0, 4)
	}

	c.preArgs = append(c.preArgs, preArgs...)
	c.argsCount -= byte(len(preArgs))
	if c.argsCount < 0 {
		panic("negative args count")
	}
}

func (c *Closure) PreArgs() []Value {
	return c.preArgs
}

func (c *Closure) SetCode(code []uint64) {
	c.code = code
}

func (c *Closure) Code() []uint64 {
	return c.code
}

func (c *Closure) SetCaller(cr Value) {
	c.caller = cr
}

func (c *Closure) Caller() Value {
	return c.caller
}

// ArgsCount returns the minimal number of arguments closure accepts
func (c *Closure) ArgsCount() int {
	return int(c.argsCount)
}

// Env returns the env inside closure
func (c *Closure) Env() *Env {
	return c.env
}

// Dup duplicates the closure
func (c *Closure) Dup() *Closure {
	cls := NewClosure(c.code, c.consts, c.env, c.argsCount, c.Yieldable(), c.Errorable(), c.receiver, c.noenvescape)
	cls.caller = c.caller
	cls.lastp = c.lastp
	cls.native = c.native
	if c.preArgs != nil {
		cls.preArgs = make([]Value, len(c.preArgs))
		copy(cls.preArgs, c.preArgs)
	}
	return cls
}

func (c *Closure) String() string {
	if c.native != nil {
		return fmt.Sprintf("closure (\n    <args: %d>\n    <curry: %d>\n    [...] native code\n)", c.argsCount, len(c.preArgs))
	}
	return "closure (\n" + c.crPrettify(4) + ")"
}

// Exec executes the closure with the given env
func (c *Closure) Exec(newEnv *Env) Value {
	if c.native == nil {

		if c.lastenv != nil {
			newEnv = c.lastenv
		} else {
			newEnv.SetParent(c.env)
		}

		v, np, yield := ExecCursor(newEnv, c.code, c.consts, c.lastp)
		if yield {
			c.lastp = np
			c.lastenv = newEnv
		} else {
			c.lastp = 0
			c.lastenv = nil
		}
		return v
	}

	// for a native closure, it doesn't have its own env,
	// so newEnv's parent is the env where this native function was called.
	return c.native(newEnv)
}
