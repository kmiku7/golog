# golog

```golang
import "github.com/kmiku7/golog"

func main() {
  fileBackend, err := golog.NewFileBackend("./log")
  if err != nil {
    panic(err)
  }
  defer fileBackend.Close()
  fileBackend.SetRotateFile(true, 24)
  
  fileBackend.Log(golog.Info, "Hello World!")
}
```
