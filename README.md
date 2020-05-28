potatolang (pol) is a Lua intepreter written in golang. It adapts most features in Lua 5.x with some extra enhancements, though its VM design is a whole different system.

## Can & Can't

- You can `return` anywhere in the function, and `continue` in a for loop.
- You can `yield` in any function as they are all coroutines, calling a yielded function again will resume it.
- You can return at most TWO values in the function. If `f` returns 2 values, you can't write `a, b, c = f(), d`.
- No builtin bitwise operators, use `bit` library instead.
- You can `+=`, `-=`, `*=` and `/=`.
- Variadic functions are completely different, you should treat and use them like Go. You can't write following code: 
    - `return ...`
    - `(function (a, ...) end)(unpack({a, b}))`
- Caller should give exact number of arguments to callee, no more no less no panic.
- You can define more than 255 variables in a function (up to 1000). Depending on the temporal variables generated by interpreter, averagely speaking you can have 800.

## Benchmarks

Refer to [here](https://github.com/coyove/potatolang/blob/master/tests/bench/perf.md).

