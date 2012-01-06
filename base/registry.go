package base

import (
  "fmt"
  "reflect"
  "sort"
  "path/filepath"
  "os"
  "strings"
)

// Many things have the following format
//   type Foo struct {
//     Defname string
//     *fooDef
//     FooInst
//   }
// Such that a Foo is something for which there can be multiple instances
// (such as a hallway, or a couch), fooDef is the data that is constant between
// all such instances, and FooInst is the data that makes each instance unique
// (location, orientation, maybe textures, etc...)  
//
// With things in this format it is convenient to have a registry structured
// like this:
//   foo_registry map[string]*fooDef
// so that a Foo can be made from a fooDef just by supplying the name of the 
// fooDef.  Given all of this the following functions are very common to all
// registries:
// GetAllFooNames() - Returns all keys in the foo_registry, in sorted order
// LoadAllFoosInDir(path string) - Finds every Foo that can be loaded in the
// specified directory and loads it into the registry.
// MakeFoo(name string) - Makes a Foo by finding the fooDef in the registry and
// embedding it in a Foo.
//
// Tags:
// The following tags can be used which will apply special processing to the
// objects when registered:
// 
// `registry:"path"` - If this object is registered with RegisterAllObjectsInDir
// and the field with this tag is a string, then the string will be joined to
// the current file's path and the resulting absolute path will replace the
// value of this field.  

var (
  registry_registry map[string]reflect.Value
)

func init() {
  registry_registry = make(map[string]reflect.Value)
}

func RemoveRegistry(name string) {
  delete(registry_registry, name)
}

// Registers a registry which must be a map from string to pointers to something
func RegisterRegistry(name string, registry interface{}) {
  mr := reflect.ValueOf(registry)
  if mr.Kind() != reflect.Map {
    panic(fmt.Sprintf("Registries must be map[string]*struct, not %v", mr.Kind()))
  }
  if mr.Type().Key().Kind() != reflect.String {
    panic(fmt.Sprintf("Registry must be a map that uses strings as keys, not %v", mr.Type().Key()))
  }
  if mr.Type().Elem().Kind() != reflect.Ptr {
    panic(fmt.Sprintf("Registry must be a map that uses pointers as values, not %v", mr.Type().Elem()))
  }
  if field,ok := mr.Type().Elem().Elem().FieldByName("Name"); !ok || field.Type.Kind() != reflect.String {
    panic(fmt.Sprintf("Registry must store values that have a Name field of type string"))
  }
  if _,ok := registry_registry[name]; ok {
    panic(fmt.Sprintf("Cannot register two registries with the same name '%s'", name))
  }
  registry_registry[name] = mr
}

// Registers object in the named registry which must have already been
// registered through RegisterRegistry().  object must be a pointer of the type
// appropriate for the named registry.
func RegisterObject(registry_name string, object interface{}) {
  reg,ok := registry_registry[registry_name]
  if !ok {
    panic(fmt.Sprintf("Tried to register an object into an unknown registry '%s'", registry_name))
  }

  obj_val := reflect.ValueOf(object)
  if obj_val.Kind() != reflect.Ptr {
    panic(fmt.Sprintf("Can only register objects as pointers, not %v", obj_val.Kind()))
  }
  if obj_val.Elem().Type() != reg.Type().Elem().Elem() {
    panic(fmt.Sprintf("Tried to register an object of type %v into the registry '%s' which stores objects of type %v", obj_val.Elem(), registry_name, reg.Type().Elem().Elem()))
  }

  // At this point we know we have the right type, and since registries can only
  // exist that store values with a field called Name of type string we don't
  // need to check for validity, we can assume it.
  object_name := obj_val.Elem().FieldByName("Name").String()
  cur_val := reg.MapIndex(reflect.ValueOf(object_name))
  if cur_val.IsValid() {
    panic(fmt.Sprintf("Tried to register an object called '%s' more than once in the registry '%s'", object_name, registry_name))
  }
  reg.SetMapIndex(reflect.ValueOf(object_name), obj_val)
}

// Loads an object using the specified registry.  object should have a field
// called Defname of type string.  This name will be used to find the def in the
// registry.  The object should also embed a field of this type which the value
// in the registry will be assigned to.
func LoadObject(registry_name string, object interface{})  {
  reg,ok := registry_registry[registry_name]
  if !ok {
    panic(fmt.Sprintf("Tried to load an object from an unknown registry '%s'", registry_name))
  }

  object_val := reflect.ValueOf(object)
  if object_val.Kind() != reflect.Ptr {
    panic("Tried to load into a value that was not a pointer")
  }

  object_name := object_val.Elem().FieldByName("Defname")
  if !object_name.IsValid() || object_name.Kind() != reflect.String {
    panic(fmt.Sprintf("Tried to load into an object that didn't have a field called Defname of type string"))
  }

  cur_val := reg.MapIndex(object_name)
  if !cur_val.IsValid() {
    panic(fmt.Sprintf("Tried to load an object, '%s', that doesn't exist in the registry '%s'", object_name.String(), registry_name))
  }
  field := object_val.Elem().FieldByName(cur_val.Elem().Type().Name())
  if !field.IsValid() {
    panic(fmt.Sprintf("Expected type %v to embed a %v", object_val.Elem().Type(), cur_val.Type()))
  }
  field.Set(cur_val)
}

// Returns a sorted list of all names in the specified registry
func GetAllNamesInRegistry(registry_name string) []string {
  reg,ok := registry_registry[registry_name]
  if !ok {
    panic(fmt.Sprintf("Requested names from an unknown registry '%s'", registry_name))
  }
  keys := reg.MapKeys()
  var names []string
  for _,key := range keys {
    names = append(names, key.String())
  }
  sort.Strings(names)
  return names
}

// Recursively decends through a value's type hierarchy and applies processing
// according to any tags that have been set on those types
func processObject(dir string, val reflect.Value, tag string) {
  switch val.Type().Kind() {
  case reflect.Ptr:
    if !val.IsNil() {
      processObject(dir, val.Elem(), tag)
    }

  case reflect.Struct:
    for i := 0; i < val.NumField(); i++ {
      processObject(dir, val.Field(i), val.Type().Field(i).Tag.Get("registry"))
    }

  case reflect.Array:
    fallthrough
  case reflect.Slice:
    for i := 0; i < val.Len(); i++ {
      processObject(dir, val.Index(i), tag)
    }

  case reflect.String:
    if tag == "path" {
      val.Set(reflect.ValueOf(filepath.Clean(filepath.Join(dir, val.String()))))
    }
  }

  // Anything that is tagged with autoload has its Load() method called if it
  // exists and has zero inputs and outputs
  if tag == "autoload" {
    load := val.MethodByName("Load")
    if !load.IsValid() && val.CanAddr() {
      load = val.Addr().MethodByName("Load")
    }
    if load.IsValid() && load.Type().NumIn() == 0 && load.Type().NumOut() == 0 {
      load.Call(nil)
    }
  }
}

// Walks recursively through the specified directory and loads all files with
// the specified suffix and loads them into the specified registry using
// RegisterObject().  format should either be "json" or "gob"
func RegisterAllObjectsInDir(registry_name,dir,suffix,format string) {
  reg,ok := registry_registry[registry_name]
  if !ok {
    panic(fmt.Sprintf("Tried to load objects into an unknown registry '%s'", registry_name))
  }
  filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
    if !info.IsDir() {
      if strings.HasSuffix(info.Name(), suffix) {
        var err error
        target := reflect.New(reg.Type().Elem().Elem())
        switch format {
        case "json":
          err = LoadJson(path, target.Interface())

        case "gob":
          err = LoadGob(path, target.Interface())

        default:
          panic(fmt.Sprintf("Can only load with format 'json' and 'gob', not '%s'", format))
        }
        if err == nil {
          RegisterObject(registry_name, target.Interface())
          processObject(path, target, "")
        }
      }
    }
    return nil
  })
}
