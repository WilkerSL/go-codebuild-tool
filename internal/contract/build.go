// Package contract turns the entities of a workspace into protobuf contracts. It writes
// the .proto files and their Go code itself, so nobody needs protoc installed.
package contract

import (
	"slices"
	"strings"

	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/WilkerSL/go-codebuild-tool/internal/naming"
	"github.com/WilkerSL/go-codebuild-tool/internal/schema"
)

type FieldType = descriptorpb.FieldDescriptorProto_Type

const (
	TypeString  = descriptorpb.FieldDescriptorProto_TYPE_STRING
	TypeBool    = descriptorpb.FieldDescriptorProto_TYPE_BOOL
	TypeInt32   = descriptorpb.FieldDescriptorProto_TYPE_INT32
	TypeInt64   = descriptorpb.FieldDescriptorProto_TYPE_INT64
	TypeFloat   = descriptorpb.FieldDescriptorProto_TYPE_FLOAT
	TypeDouble  = descriptorpb.FieldDescriptorProto_TYPE_DOUBLE
	TypeBytes   = descriptorpb.FieldDescriptorProto_TYPE_BYTES
	TypeMessage = descriptorpb.FieldDescriptorProto_TYPE_MESSAGE
)

const (
	Timestamp = ".google.protobuf.Timestamp"
	Empty     = ".google.protobuf.Empty"
)

type Field struct {
	Name     string
	GoName   string
	Number   int32
	Type     FieldType
	TypeName string
	Optional bool
	Repeated bool
	Source   *schema.Field
}

type Message struct {
	Name     string
	FullName string
	Fields   []*Field
	Reserved []int32
}

func (m *Message) Field(name string) *Field {
	for _, f := range m.Fields {
		if f.Name == name {
			return f
		}
	}
	return nil
}

type Method struct {
	Name   string
	Input  string
	Output string
	// Safe marks a call without side effects, which clients may retry.
	Safe bool
}

type Service struct {
	Name    string
	Methods []*Method
}

type File struct {
	Service  *schema.Service
	Path     string
	Package  string
	GoImport string
	GoName   string
	Imports  []string
	Messages []*Message
	Services []*Service
	registry *Registry
	byName   map[string]*Message
	isCommon bool
}

func (f *File) Message(name string) *Message {
	return f.byName[name]
}

type Set struct {
	Module   string
	Common   *File
	Files    []*File
	registry *Registry
}

func (s *Set) File(service string) *File {
	for _, f := range s.Files {
		if f.Service != nil && f.Service.Name == service {
			return f
		}
	}
	return nil
}

const (
	CommonPackage    = "common.v1"
	ProblemMessage   = ".common.v1.Problem"
	ExistsResponse   = ".common.v1.ExistsResponse"
	commonPath       = "common/v1/common.proto"
	timestampPath    = "google/protobuf/timestamp.proto"
	emptyPath        = "google/protobuf/empty.proto"
	ContractsDir     = "contracts"
	ContractRegistry = "contracts/codebuild.contracts.json"
)

func Build(module string, m *schema.Model, registry *Registry) *Set {
	if registry == nil {
		registry = NewRegistry()
	}
	s := &Set{Module: module, registry: registry}
	s.Common = s.newFile(nil, "common", commonPath, CommonPackage)
	s.Common.isCommon = true
	violation := s.Common.message("FieldViolation", field("field", TypeString), field("message", TypeString))
	s.Common.message("Problem", field("status", TypeInt32), &Field{Name: "errors", Type: TypeMessage, TypeName: violation.FullName, Repeated: true})
	s.Common.message("ExistsResponse", field("exists", TypeBool))
	for _, svc := range m.Services {
		f := s.newFile(svc, svc.Name, svc.Name+"/v1/"+svc.Name+".proto", svc.Name+".v1")
		for _, e := range svc.Entities {
			f.entity(m, e)
		}
		if auth := m.AuthEntity(); auth != nil && auth.Service == svc {
			f.auth(auth)
		}
		f.Imports = []string{commonPath, emptyPath, timestampPath}
		s.Files = append(s.Files, f)
	}
	keep := map[string]bool{}
	for _, f := range append([]*File{s.Common}, s.Files...) {
		for _, msg := range f.Messages {
			keep[strings.TrimPrefix(msg.FullName, ".")] = true
		}
	}
	registry.prune(keep)
	return s
}

func (s *Set) newFile(svc *schema.Service, name, path, pkg string) *File {
	return &File{
		Service:  svc,
		Path:     path,
		Package:  pkg,
		GoImport: s.Module + "/" + ContractsDir + "/" + name + "/v1",
		GoName:   name + "v1",
		registry: s.registry,
		byName:   map[string]*Message{},
	}
}

func field(name string, t FieldType) *Field {
	return &Field{Name: name, Type: t}
}

func (f *File) message(name string, fields ...*Field) *Message {
	full := "." + f.Package + "." + name
	names := make([]string, len(fields))
	types := make([]string, len(fields))
	for i, fd := range fields {
		names[i], types[i] = fd.Name, wireType(fd)
	}
	numbers, reserved := f.registry.assign(strings.TrimPrefix(full, "."), names, types)
	for _, fd := range fields {
		fd.Number = numbers[fd.Name]
	}
	msg := &Message{Name: name, FullName: full, Fields: fields, Reserved: reserved}
	f.Messages = append(f.Messages, msg)
	f.byName[name] = msg
	return msg
}

// wireType names what a field looks like on the wire; optional does not count,
// because proto3 encodes optional and plain fields the same way.
func wireType(f *Field) string {
	t := strings.ToLower(strings.TrimPrefix(f.Type.String(), "TYPE_"))
	if f.Type == TypeMessage {
		t = strings.TrimPrefix(f.TypeName, ".")
	}
	if f.Repeated {
		t = "repeated " + t
	}
	return t
}

func ProtoName(goName string) string {
	return naming.Snake(goName)
}

func scalar(k schema.Kind) (FieldType, string) {
	switch k {
	case schema.KindBool:
		return TypeBool, ""
	case schema.KindInt16, schema.KindInt32:
		return TypeInt32, ""
	case schema.KindInt, schema.KindInt64:
		return TypeInt64, ""
	case schema.KindFloat32:
		return TypeFloat, ""
	case schema.KindFloat64:
		return TypeDouble, ""
	case schema.KindBytes:
		return TypeBytes, ""
	case schema.KindTime:
		return TypeMessage, Timestamp
	}
	return TypeString, ""
}

func fromField(name string, sf *schema.Field, presence bool) *Field {
	t, typeName := scalar(sf.Kind)
	return &Field{Name: name, Type: t, TypeName: typeName, Optional: presence && t != TypeMessage, Source: sf}
}

func idField(name string, pk *schema.Field) *Field {
	t, typeName := scalar(pk.Kind)
	return &Field{Name: name, Type: t, TypeName: typeName, Source: pk}
}

type RelationRPC struct {
	Source *schema.Entity
	Rel    *schema.Relation
	FK     *schema.Field
	List   string
	Link   string
	Unlink string
}

func Relations(m *schema.Model, target *schema.Entity) []RelationRPC {
	var out []RelationRPC
	for _, source := range m.Entities {
		for _, r := range source.Relations {
			if r.Target != target.Name || source.PK() == nil {
				continue
			}
			rpc := RelationRPC{Source: source, Rel: r, List: "List" + source.Name + r.Name}
			switch r.Kind {
			case schema.RelationHasMany:
				rpc.FK = target.Field(r.FK)
				if rpc.FK == nil {
					continue
				}
			case schema.RelationManyToMany:
				if !target.ReadOnly {
					rpc.Link, rpc.Unlink = "Link"+source.Name+r.Name, "Unlink"+source.Name+r.Name
				}
			}
			out = append(out, rpc)
		}
	}
	return out
}

func (f *File) entity(m *schema.Model, e *schema.Entity) {
	pk := e.PK()
	var response []*Field
	for _, sf := range e.Fields {
		if !sf.OmitResponse {
			response = append(response, fromField(ProtoName(sf.Name), sf, sf.Nullable))
		}
	}
	entity := f.message(e.Name, response...)
	page := f.message(e.Name+"Page",
		&Field{Name: "items", Type: TypeMessage, TypeName: entity.FullName, Repeated: true},
		field("limit", TypeInt32), field("offset", TypeInt32), &Field{Name: "total", Type: TypeInt64, Optional: true},
		&Field{Name: "next_cursor", Type: TypeString, Optional: true})
	filters := func(skip *schema.Field) []*Field {
		fields := []*Field{field("limit", TypeInt32), field("offset", TypeInt32)}
		for _, sf := range e.FilterFields() {
			if sf != skip {
				fields = append(fields, fromField(ProtoName(sf.Name), sf, true))
			}
		}
		return append(fields, field("cursor", TypeString))
	}
	get := f.message(e.Name+"GetRequest", idField("id", pk))
	list := f.message(e.Name+"ListRequest", filters(nil)...)
	exists := f.message(e.Name+"ExistsRequest", idField("id", pk))
	svc := &Service{Name: e.Name + "Service"}
	svc.Methods = append(svc.Methods,
		&Method{Name: "Get", Input: get.FullName, Output: entity.FullName, Safe: true},
		&Method{Name: "List", Input: list.FullName, Output: page.FullName, Safe: true},
	)
	if !e.ReadOnly {
		var create []*Field
		for _, sf := range e.Fields {
			if sf.InCreate() {
				create = append(create, fromField(ProtoName(sf.InputName()), sf, true))
			}
		}
		createMsg := f.message(e.Name+"CreateRequest", create...)
		svc.Methods = append(svc.Methods, &Method{Name: "Create", Input: createMsg.FullName, Output: entity.FullName})
		if updateFields := e.UpdateFields(); len(updateFields) > 0 {
			update := []*Field{idField("id", pk)}
			for _, sf := range updateFields {
				update = append(update, fromField(ProtoName(sf.Name), sf, true))
			}
			updateMsg := f.message(e.Name+"UpdateRequest", update...)
			svc.Methods = append(svc.Methods, &Method{Name: "Update", Input: updateMsg.FullName, Output: entity.FullName})
			patchMsg := f.message(e.Name+"PatchRequest",
				&Field{Name: "update", Type: TypeMessage, TypeName: updateMsg.FullName},
				&Field{Name: "fields", Type: TypeString, Repeated: true})
			svc.Methods = append(svc.Methods, &Method{Name: "Patch", Input: patchMsg.FullName, Output: entity.FullName})
		}
		del := f.message(e.Name+"DeleteRequest", idField("id", pk))
		svc.Methods = append(svc.Methods, &Method{Name: "Delete", Input: del.FullName, Output: Empty})
	}
	svc.Methods = append(svc.Methods, &Method{Name: "Exists", Input: exists.FullName, Output: ExistsResponse, Safe: true})
	for _, rel := range Relations(m, e) {
		fields := append([]*Field{idField("source_id", rel.Source.PK())}, filters(rel.FK)...)
		req := f.message(rel.List+"Request", fields...)
		svc.Methods = append(svc.Methods, &Method{Name: rel.List, Input: req.FullName, Output: page.FullName, Safe: true})
		if rel.Link != "" {
			link := f.message(rel.Link+"Request", idField("source_id", rel.Source.PK()), idField("id", pk))
			unlink := f.message(rel.Unlink+"Request", idField("source_id", rel.Source.PK()), idField("id", pk))
			svc.Methods = append(svc.Methods,
				&Method{Name: rel.Link, Input: link.FullName, Output: Empty},
				&Method{Name: rel.Unlink, Input: unlink.FullName, Output: Empty})
		}
	}
	f.Services = append(f.Services, svc)
}

func (f *File) auth(e *schema.Entity) {
	principal := f.message("Principal", field("id", TypeString), field("role", TypeString), field("method", TypeString), field("credential_id", TypeString))
	authenticate := f.message("AuthenticateRequest", field("token", TypeString))
	login := f.message("LoginRequest", field("identity", TypeString), field("password", TypeString))
	tokenFields := []*Field{field("access_token", TypeString), field("token_type", TypeString), {Name: "expires_at", Type: TypeMessage, TypeName: Timestamp}}
	if e.HasMethod(schema.MethodJWT) {
		tokenFields = append(tokenFields, field("refresh_token", TypeString), &Field{Name: "refresh_expires_at", Type: TypeMessage, TypeName: Timestamp})
	}
	tokens := f.message("Tokens", tokenFields...)
	change := f.message("ChangePasswordRequest", field("current_password", TypeString), field("new_password", TypeString))
	account := f.Message(e.Name)
	svc := &Service{Name: "AuthService"}
	svc.Methods = append(svc.Methods,
		&Method{Name: "Authenticate", Input: authenticate.FullName, Output: principal.FullName, Safe: true},
		&Method{Name: "Login", Input: login.FullName, Output: tokens.FullName},
	)
	if e.Register {
		svc.Methods = append(svc.Methods, &Method{Name: "Register", Input: f.Message(e.Name + "CreateRequest").FullName, Output: account.FullName})
	}
	if e.HasMethod(schema.MethodJWT) {
		refresh := f.message("RefreshRequest", field("refresh_token", TypeString))
		svc.Methods = append(svc.Methods, &Method{Name: "Refresh", Input: refresh.FullName, Output: tokens.FullName})
	}
	svc.Methods = append(svc.Methods,
		&Method{Name: "Logout", Input: Empty, Output: Empty},
		&Method{Name: "Me", Input: Empty, Output: account.FullName, Safe: true},
		&Method{Name: "ChangePassword", Input: change.FullName, Output: Empty},
	)
	limit := f.message("RateLimitRequest", field("key", TypeString), field("limit", TypeInt32), field("window_millis", TypeInt64))
	limited := f.message("RateLimitResponse", field("retry_after_millis", TypeInt64))
	resetLimit := f.message("RateLimitResetRequest", field("key", TypeString))
	svc.Methods = append(svc.Methods,
		&Method{Name: "HitRateLimit", Input: limit.FullName, Output: limited.FullName},
		&Method{Name: "ResetRateLimit", Input: resetLimit.FullName, Output: Empty},
	)
	if e.HasMethod(schema.MethodAPIKey) {
		keyFields := func(extra ...*Field) []*Field {
			fields := []*Field{field("id", TypeString), field("name", TypeString), field("prefix", TypeString),
				{Name: "created_at", Type: TypeMessage, TypeName: Timestamp}, {Name: "expires_at", Type: TypeMessage, TypeName: Timestamp}}
			return append(fields, extra...)
		}
		key := f.message("APIKey", keyFields()...)
		created := f.message("CreatedAPIKey", keyFields(field("key", TypeString))...)
		createKey := f.message("APIKeyCreateRequest", field("name", TypeString), &Field{Name: "expires_at", Type: TypeMessage, TypeName: Timestamp})
		keyList := f.message("APIKeyList", &Field{Name: "items", Type: TypeMessage, TypeName: key.FullName, Repeated: true})
		deleteKey := f.message("APIKeyDeleteRequest", field("id", TypeString))
		svc.Methods = append(svc.Methods,
			&Method{Name: "CreateAPIKey", Input: createKey.FullName, Output: created.FullName},
			&Method{Name: "ListAPIKeys", Input: Empty, Output: keyList.FullName, Safe: true},
			&Method{Name: "DeleteAPIKey", Input: deleteKey.FullName, Output: Empty},
		)
	}
	f.Services = append(f.Services, svc)
}

func (s *Set) All() []*File {
	return append([]*File{s.Common}, s.Files...)
}

func (s *Set) Registry() *Registry {
	return s.registry
}

// SafeMethods returns the full names, such as /catalog.v1.ProductService/Get,
// of every generated call without side effects, sorted.
func (s *Set) SafeMethods() []string {
	var names []string
	for _, f := range s.All() {
		for _, svc := range f.Services {
			for _, m := range svc.Methods {
				if m.Safe {
					names = append(names, "/"+f.Package+"."+svc.Name+"/"+m.Name)
				}
			}
		}
	}
	slices.Sort(names)
	return names
}

func sortedImports(imports []string) []string {
	out := slices.Clone(imports)
	slices.Sort(out)
	return out
}
