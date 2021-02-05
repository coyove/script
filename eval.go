package script

import (
	"bytes"
	"fmt"
	"math"
	"strconv"
	"sync/atomic"
)

type stacktrace struct {
	cursor      uint32
	stackOffset uint32
	cls         *Func
}

// ExecError represents the runtime error
type ExecError struct {
	r      interface{}
	stacks []stacktrace
}

func (e *ExecError) Error() string {
	msg := bytes.Buffer{}
	msg.WriteString("stacktrace:\n")
	for i := len(e.stacks) - 1; i >= 0; i-- {
		r := e.stacks[i]
		src := uint32(0)
		for i := 0; i < len(r.cls.Code.Pos); {
			var opx uint32 = math.MaxUint32
			ii, op, line := r.cls.Code.Pos.read(i)
			if ii < len(r.cls.Code.Pos)-1 {
				_, opx, _ = r.cls.Code.Pos.read(ii)
			}
			if r.cursor >= op && r.cursor < opx {
				src = line
				break
			}
			if r.cursor < op && i == 0 {
				src = line
				break
			}
			i = ii
		}
		// the recorded cursor was advanced by 1 already
		msg.WriteString(fmt.Sprintf("%s at line %d (cursor: %d)\n", r.cls.Name, src, r.cursor-1))
	}
	msg.WriteString("root panic:\n")
	msg.WriteString(fmt.Sprintf("%v\n", e.r))
	return msg.String()
}

// InternalExecCursorLoop executes 'K' under 'Env' from the given start 'cursor'
func InternalExecCursorLoop(env Env, K *Func, cursor uint32) Value {
	var stackEnv = env
	var retStack []stacktrace

	stackEnv.StackOffset = uint32(len(*env.stack))

	defer func() {
		if r := recover(); r != nil {
			rr := stacktrace{
				cursor: cursor,
				cls:    K,
			}

			if re, ok := r.(*ExecError); ok {
				retStack = append(retStack, rr)
				re.stacks = append(retStack, re.stacks...)
				panic(re)
			} else {
				e := &ExecError{}
				e.r = r // root panic
				e.stacks = make([]stacktrace, len(retStack)+1)
				copy(e.stacks, retStack)
				e.stacks[len(e.stacks)-1] = rr
				panic(e)
			}
		}
	}()

	for {
		if env.Global.Deadline != 0 {
			if atomic.LoadInt64(&now) > env.Global.Deadline {
				panicf("timeout")
			}
		}

		v := K.Code.Code[cursor]
		cursor++
		bop, opa, opb := splitInst(v)

		switch bop {
		case OpSet:
			env._set(opa, env._get(opb))
			// if b.Type() == VString {
			// 	b.String()
			// }
		case OpInc:
			vaf, vai, vaIsInt := env._get(opa).Expect(VNumber).Num()
			vbf, vbi, vbIsInt := env._get(opb).Expect(VNumber).Num()
			if vaIsInt && vbIsInt {
				env.A = Int(vai + vbi)
			} else {
				env.A = Float(vaf + vbf)
			}
			env._set(opa, env.A)
		case OpConcat:
			if va, vb := env._get(opa), env._get(opb); va.Type()+vb.Type() == _StrStr {
				env.A = concat(env.Global, va._str(), vb._str())
			} else if va.Type() == VString && vb.Type() == VNumber {
				if vbf, vbi, vbIsInt := vb.Num(); vbIsInt {
					env.A = concat(env.Global, va._str(), strconv.FormatInt(vbi, 10))
				} else {
					env.A = concat(env.Global, va._str(), strconv.FormatFloat(vbf, 'f', 0, 64))
				}
			} else if vb.Type() == VString && va.Type() == VNumber {
				if vaf, vai, vaIsInt := va.Num(); vaIsInt {
					env.A = concat(env.Global, strconv.FormatInt(vai, 10), vb._str())
				} else {
					env.A = concat(env.Global, strconv.FormatFloat(vaf, 'f', 0, 64), vb._str())
				}
			} else {
				va, vb = va.Expect(VString), vb.Expect(VString)
			}
		case OpAdd:
			if va, vb := env._get(opa), env._get(opb); va.Type()+vb.Type() == _NumNum {
				vaf, vai, vaIsInt := va.Num()
				vbf, vbi, vbIsInt := vb.Num()
				if vaIsInt && vbIsInt {
					env.A = Int(vai + vbi)
				} else {
					env.A = Float(vaf + vbf)
				}
			} else if va.Type() == VNumber && vb.Type() == VString {
				vaf, vai, vaIsInt := va.Num()
				if vaIsInt {
					vbi, _ := strconv.ParseInt(vb._str(), 0, 64)
					env.A = Int(vai + vbi)
				} else {
					vbf, _ := strconv.ParseFloat(vb._str(), 64)
					env.A = Float(vaf + vbf)
				}
			} else {
				va, vb = va.Expect(VNumber), vb.Expect(VNumber)
			}
		case OpSub:
			if va, vb := env._get(opa), env._get(opb); va.Type()+vb.Type() == _NumNum {
				vaf, vai, vaIsInt := va.Num()
				vbf, vbi, vbIsInt := vb.Num()
				if vaIsInt && vbIsInt {
					env.A = Int(vai - vbi)
				} else {
					env.A = Float(vaf - vbf)
				}
			} else {
				va, vb = va.Expect(VNumber), vb.Expect(VNumber)
			}
		case OpMul:
			if va, vb := env._get(opa), env._get(opb); va.Type()+vb.Type() == _NumNum {
				vaf, vai, vaIsInt := va.Num()
				vbf, vbi, vbIsInt := vb.Num()
				if vaIsInt && vbIsInt {
					env.A = Int(vai * vbi)
				} else {
					env.A = Float(vaf * vbf)
				}
			} else {
				va, vb = va.Expect(VNumber), vb.Expect(VNumber)
			}
		case OpDiv:
			if va, vb := env._get(opa), env._get(opb); va.Type()+vb.Type() == _NumNum {
				vaf, vai, vaIsInt := va.Num()
				vbf, vbi, vbIsInt := vb.Num()
				if vaIsInt && vbIsInt && vai%vbi == 0 {
					env.A = Int(vai / vbi)
				} else {
					env.A = Float(vaf / vbf)
				}
			} else {
				va, vb = va.Expect(VNumber), vb.Expect(VNumber)
			}
		case OpIDiv:
			env.A = Int(env._get(opa).ExpectMsg(VNumber, "idiv").Int() / env._get(opb).ExpectMsg(VNumber, "idiv").Int())
		case OpMod:
			if va, vb := env._get(opa), env._get(opb); va.Type()+vb.Type() == _NumNum {
				vaf, vai, vaIsInt := va.Num()
				vbf, vbi, vbIsInt := vb.Num()
				if vaIsInt && vbIsInt {
					env.A = Int(vai % vbi)
				} else {
					env.A = Float(math.Remainder(vaf, vbf))
				}
			} else {
				va, vb = va.Expect(VNumber), vb.Expect(VNumber)
			}
		case OpEq:
			env.A = Bool(env._get(opa).Equal(env._get(opb)))
		case OpNeq:
			env.A = Bool(!env._get(opa).Equal(env._get(opb)))
		case OpLess:
			switch va, vb := env._get(opa), env._get(opb); va.Type() + vb.Type() {
			case _NumNum:
				vaf, vai, vaIsInt := va.Num()
				vbf, vbi, vbIsInt := vb.Num()
				if vaIsInt && vbIsInt {
					env.A = Bool(vai < vbi)
				} else {
					env.A = Bool(vaf < vbf)
				}
			case _StrStr:
				env.A = Bool(va._str() < vb._str())
			default:
				va, vb = va.Expect(VNumber), vb.Expect(VNumber)
			}
		case OpLessEq:
			switch va, vb := env._get(opa), env._get(opb); va.Type() + vb.Type() {
			case _NumNum:
				vaf, vai, vaIsInt := va.Num()
				vbf, vbi, vbIsInt := vb.Num()
				if vaIsInt && vbIsInt {
					env.A = Bool(vai <= vbi)
				} else {
					env.A = Bool(vaf <= vbf)
				}
			case _StrStr:
				env.A = Bool(va._str() <= vb._str())
			default:
				va, vb = va.Expect(VNumber), vb.Expect(VNumber)
			}
		case OpNot:
			env.A = Bool(env._get(opa).IsFalse())
		case OpPow:
			switch va, vb := env._get(opa), env._get(opb); va.Type() + vb.Type() {
			case _NumNum:
				vaf, vai, vaIsInt := va.Num()
				vbf, vbi, vbIsInt := vb.Num()
				if vaIsInt && vbIsInt && vbi >= 1 {
					env.A = Int(ipow(vai, vbi))
				} else {
					env.A = Float(math.Pow(vaf, vbf))
				}
			default:
				va, vb = va.Expect(VNumber), vb.Expect(VNumber)
			}
		case OpLen:
			switch v := env._get(opa); v.Type() {
			case VString:
				env.A = Float(float64(len(v._str())))
			case VArray:
				env.A = Int(int64(v.Array().Len()))
			case VFunction:
				env.A = Float(float64(v.Function().NumParams))
			default:
				env.A = Int(int64(reflectLen(v.Interface())))
			}
		case OpList:
			env.Global.DecrDeadsize(int64(stackEnv.Size()) * ValueSize)
			env.A = Array(append([]Value{}, stackEnv.Stack()...)...)
			stackEnv.Clear()
		case OpGStore:
			env.A = env._get(opb)
			env.Global.GStore(env._get(opa).ExpectMsg(VString, "gstore")._str(), env.A)
		case OpGLoad:
			env.A = env.Global.GLoad(env._get(opa).ExpectMsg(VString, "gload")._str())
		case OpStore:
			subject, v := env._get(opa), env._get(opb)
			switch subject.Type() {
			case VArray:
				if subject.Array().Put1(env.A.ExpectMsg(VNumber, "store").Int(), v) {
					env.Global.DecrDeadsize(ValueSize)
				}
			case VInterface:
				reflectStore(subject.Interface(), env.A, v)
			default:
				subject = subject.Expect(VArray)
			}
			env.A = v
		case OpSlice:
			subject := env._get(opa)
			start, end := env.A.ExpectMsg(VNumber, "slice").Int(), env._get(opb).ExpectMsg(VNumber, "slice").Int()
			switch subject.Type() {
			case VArray:
				env.A = Array(subject.Array().Slice1(start, end)...)
			case VString:
				s := subject._str()
				start, end := sliceInRange(start, end, len(s))
				env.A = String(s[start:end])
			case VInterface:
				env.A = Interface(reflectSlice(subject.Interface(), start, end))
			default:
				subject = subject.Expect(VArray)
			}
		case OpLoad:
			switch a := env._get(opa); a.Type() {
			case VArray:
				env.A = a.Array().Get1(env._get(opb).ExpectMsg(VNumber, "load").Int())
			case VInterface:
				env.A = reflectLoad(a.Interface(), env._get(opb))
			case VString:
				if idx, s := env._get(opb).ExpectMsg(VNumber, "load").Int(), a._str(); idx >= 1 && idx <= int64(len(s)) {
					env.A = Int(int64(s[idx-1]))
				}
			default:
				a = a.Expect(VArray)
			}
		case OpPush:
			v := env._get(opa)
			if v.Type() == VArray && v.Array().Unpacked {
				*stackEnv.stack = append(*stackEnv.stack, v.Array().Underlay()...)
			} else {
				stackEnv.Push(v)
			}
		case OpRet:
			v := env._get(opa)
			if len(retStack) == 0 {
				return v
			}
			// Return upper stack
			r := retStack[len(retStack)-1]
			cursor = r.cursor
			K = r.cls
			env.StackOffset = r.stackOffset
			env.A = v
			*env.stack = (*env.stack)[:env.StackOffset+uint32(r.cls.StackSize)]
			stackEnv.StackOffset = uint32(len(*env.stack))
			retStack = retStack[:len(retStack)-1]
		case OpLoadFunc:
			env.A = Function(env.Global.Functions[opa])
		case OpCallMap:
			cls := env._get(opa).ExpectMsg(VFunction, "callmap").Function()
			m := make(map[string]Value, stackEnv.Size()/2)
			for i := 0; i < stackEnv.Size(); i += 2 {
				a := stackEnv.Stack()[i]
				if a.IsNil() {
					continue
				}
				var name string
				if a.Type() == VNumber && a.Int() < int64(len(cls.Params)) {
					name = cls.Params[a.Int()]
				} else {
					name = a.String()
				}
				if _, ok := m[name]; ok {
					panicf("call: duplicated parameter: %q", name)
				}
				m[name] = stackEnv.Get(i + 1)
			}
			stackEnv.Clear()
			for i := byte(0); i < cls.NumParams; i++ {
				if int(i) < len(cls.Params) {
					stackEnv.Push(m[cls.Params[i]])
				} else {
					stackEnv.Push(Value{})
				}
			}
			stackEnv.A = Interface(m)
			fallthrough
		case OpCall:
			cls := env._get(opa).ExpectMsg(VFunction, "call").Function()

			if cls.Native != nil {
				stackEnv.Global = env.Global
				stackEnv.NativeSource = cls
				if cls.IsDebug {
					stackEnv.Debug = &debugInfo{
						Caller:     K,
						Cursor:     cursor,
						Stacktrace: append(retStack, stacktrace{cls: K, cursor: cursor}),
					}
					cls.Native(&stackEnv)
					stackEnv.Debug = nil
				} else {
					cls.Native(&stackEnv)
				}
				env.A = stackEnv.A
				stackEnv.NativeSource = nil
				stackEnv.Clear()
			} else {
				stackEnv.growZero(int(cls.StackSize))

				last := stacktrace{
					cls:         K,
					cursor:      cursor,
					stackOffset: uint32(env.StackOffset),
				}

				// Switch 'env' to 'stackEnv' and clear 'stackEnv'
				cursor = 0
				K = cls
				env.StackOffset = stackEnv.StackOffset
				env.Global = cls.loadGlobal
				env.A = stackEnv.A

				if opb == 0 {
					retStack = append(retStack, last)
				}

				if env.Global.MaxCallStackSize > 0 && int64(len(retStack)) > env.Global.MaxCallStackSize {
					panicf("call stack overflow, max: %d", env.Global.MaxCallStackSize)
				}

				stackEnv.StackOffset = uint32(len(*env.stack))
			}
		case OpJmp:
			cursor = uint32(int32(cursor) + int32(v&0xffffff) - 1<<23)
		case OpIfNot:
			if env.A.IsFalse() {
				cursor = uint32(int32(cursor) + int32(v&0xffffff) - 1<<23)
			}
		case OpIf:
			if !env.A.IsFalse() {
				cursor = uint32(int32(cursor) + int32(v&0xffffff) - 1<<23)
			}
		}
	}
}

func concat(p *Program, a, b string) Value {
	p.DecrDeadsize(int64(len(a) + len(b)))
	return String(a + b)
}
