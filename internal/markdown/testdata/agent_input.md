# Agent formatting fixture

This paragraph has **bold**, *italic*, and `inline code`.
It also has a second line in the same paragraph.

## Bullet-like lines
- first item
- second item with `code`
- third item with **bold** text

### Escapes and punctuation
These characters should stay readable: _ * [ ] ( ) ~ ` > # + - = | { } . !
Paths and values: `C:\temp\demo`, `a_b`, `x.y`, and `hello-world`.

### Go example
```go
package main

import "fmt"

func main() {
	fmt.Println("hello")
}
```
```diff
- old_value := call("a_b")
+ new_value := call("x.y")
```

Final paragraph after the code blocks.
