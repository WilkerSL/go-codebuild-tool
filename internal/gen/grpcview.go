package gen

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/WilkerSL/go-codebuild-tool/internal/contract"
	"github.com/WilkerSL/go-codebuild-tool/internal/naming"
	"github.com/WilkerSL/go-codebuild-tool/internal/schema"
)

type grpcOp struct {
	Name      string
	Request   string
	Authorize string
	Owner     string
}

type peerClient struct {
	Field   string
	Type    string
	Service string
	Alias   string
	Import  string
	Entity  *schema.Entity
	Factory string
}

type remoteCheck struct {
	Client   string
	Alias    string
	Request  string
	Field    string
	JSON     string
	Value    string
	Nullable bool
}

type grpcRelation struct {
	*relationView
	List         grpcOp
	Link         grpcOp
	Unlink       grpcOp
	ParamsFunc   string
	ParamsBody   string
	SourceParse  string
	SourceCheck  *remoteCheck
	LinkRequest  string
	UnlinkReq    string
	TargetIDExpr string
}

type grpcView struct {
	Alias        string
	Import       string
	Service      string
	Message      string
	Page         string
	ToMessage    string
	Get          grpcOp
	List         grpcOp
	Create       grpcOp
	Update       grpcOp
	Delete       grpcOp
	Exists       grpcOp
	IDParse      string
	CreateFrom   string
	UpdateFrom   string
	ListFrom     string
	Checks       []remoteCheck
	Peers        []*peerClient
	PeerImports  []goImport
	Relations    []*grpcRelation
	RegisterFrom string
}

func protoValue(f *schema.Field, expr string) string {
	switch {
	case f.Kind == schema.KindUUID && f.Nullable:
		return "grpcx.UUIDString(" + expr + ")"
	case f.Kind == schema.KindUUID:
		return expr + ".String()"
	case f.Kind == schema.KindTime && f.Nullable:
		return "grpcx.NullTimestamp(" + expr + ")"
	case f.Kind == schema.KindTime:
		return "grpcx.Timestamp(" + expr + ")"
	case f.Kind == schema.KindInt16 && f.Nullable:
		return "grpcx.Convert[int16, int32](" + expr + ")"
	case f.Kind == schema.KindInt16:
		return "int32(" + expr + ")"
	case f.Kind == schema.KindInt && f.Nullable:
		return "grpcx.Convert[int, int64](" + expr + ")"
	case f.Kind == schema.KindInt:
		return "int64(" + expr + ")"
	case f.Kind == schema.KindBytes && f.Nullable:
		return "grpcx.Bytes(" + expr + ")"
	}
	return expr
}

func requestValue(f *schema.Field, expr, json string) string {
	switch f.Kind {
	case schema.KindTime:
		return "grpcx.TimeString(" + expr + ")"
	case schema.KindInt16:
		return "grpcx.Narrow[int32, int16](" + expr + ", " + strconv.Quote(json) + ", &errs)"
	case schema.KindInt:
		return "grpcx.Convert[int64, int](" + expr + ")"
	case schema.KindBytes:
		return "grpcx.BytesPtr(" + expr + ")"
	}
	return expr
}

func filterValue(f *schema.Field, expr string) string {
	switch f.Kind {
	case schema.KindUUID:
		return "grpcx.ParseNullUUID(" + expr + ", " + strconv.Quote(f.JSON) + ", &errs)"
	case schema.KindInt16:
		return "grpcx.Narrow[int32, int16](" + expr + ", " + strconv.Quote(f.JSON) + ", &errs)"
	case schema.KindInt:
		return "grpcx.Convert[int64, int](" + expr + ")"
	}
	return expr
}

func idParse(pk *schema.Field, expr, name, target string) string {
	switch pk.Kind {
	case schema.KindUUID:
		return fmt.Sprintf("%s, err := grpcx.ParseUUID(%s, %q)\nif err != nil {\nreturn nil, err\n}", target, expr, name)
	case schema.KindInt:
		return fmt.Sprintf("%s := int(%s)", target, expr)
	}
	return fmt.Sprintf("%s := %s", target, expr)
}

func idProto(pk *schema.Field, expr string) string {
	switch pk.Kind {
	case schema.KindUUID:
		return expr + ".String()"
	case schema.KindInt:
		return "int64(" + expr + ")"
	}
	return expr
}

func authorizeCode(policy schema.Policy, owned, enabled bool) string {
	if !enabled || (policy.Public && !owned) {
		return ""
	}
	args := ""
	for _, r := range policy.Roles {
		args += ", " + strconv.Quote(r)
	}
	if owned {
		return "caller, err := grpcx.Authorize(ctx" + args + ")\nif err != nil {\nreturn nil, err\n}"
	}
	return "if _, err := grpcx.Authorize(ctx" + args + "); err != nil {\nreturn nil, err\n}"
}

func messageFields(msg *contract.Message) map[string]*contract.Field {
	out := map[string]*contract.Field{}
	for _, f := range msg.Fields {
		out[f.Name] = f
	}
	return out
}

func newGRPCView(dv *domainView, m *schema.Model, file *contract.File, enabled bool, peers map[string]*peerClient, peerOf func(*schema.Entity) *peerClient) *grpcView {
	e := dv.Entity
	pk := e.PK()
	gv := &grpcView{
		Alias:   file.GoName,
		Import:  file.GoImport,
		Service: e.Name + "Service",
		Message: e.Name,
		Page:    e.Name + "Page",
	}
	owned := e.Owner() != nil
	ownerArg := ""
	if owned {
		ownerArg = "caller.ID, "
	}
	op := func(name, request string, policy schema.Policy) grpcOp {
		return grpcOp{Name: name, Request: request, Authorize: authorizeCode(policy, owned, enabled), Owner: ownerArg}
	}
	gv.Get = op("Get", e.Name+"GetRequest", e.Policy(schema.OpRead))
	gv.List = op("List", e.Name+"ListRequest", e.Policy(schema.OpRead))
	// Exists only serves other services checking a reference, never the gateway, so it
	// ignores the read roles. Owned rows are still scoped to the caller.
	gv.Exists = op("Exists", e.Name+"ExistsRequest", schema.Policy{Public: true})
	gv.Create = op("Create", e.Name+"CreateRequest", e.Policy(schema.OpCreate))
	gv.Update = op("Update", e.Name+"UpdateRequest", e.Policy(schema.OpUpdate))
	gv.Delete = op("Delete", e.Name+"DeleteRequest", e.Policy(schema.OpDelete))
	gv.IDParse = idParse(pk, "req.GetId()", "id", "id")

	var b strings.Builder
	b.WriteString("&" + gv.Alias + "." + e.Name + "{\n")
	for _, fd := range file.Message(e.Name).Fields {
		fmt.Fprintf(&b, "%s: %s,\n", fd.GoName, protoValue(fd.Source, "item."+fd.Source.Name))
	}
	b.WriteString("}")
	gv.ToMessage = b.String()

	conversion := func(message, target string, fields []*fieldView, update bool) string {
		msg := file.Message(message)
		if msg == nil {
			return ""
		}
		byName := messageFields(msg)
		var c strings.Builder
		c.WriteString("var errs validation.Errors\nr := " + target + "{\n")
		for _, fv := range fields {
			name := contract.ProtoName(fv.F.InputName())
			if update {
				name = contract.ProtoName(fv.F.Name)
			}
			fd := byName[name]
			if fd == nil {
				continue
			}
			fmt.Fprintf(&c, "%s: %s,\n", fv.Input, requestValue(fv.F, "req."+fd.GoName, fv.InputJSON))
		}
		c.WriteString("}\n")
		return c.String()
	}
	if !e.ReadOnly {
		gv.CreateFrom = conversion(e.Name+"CreateRequest", "createRequest", dv.Create, false)
		if dv.HasUpdate {
			gv.UpdateFrom = conversion(e.Name+"UpdateRequest", "updateRequest", dv.Update, true)
		}
		if e.Register {
			var register []*fieldView
			for _, fv := range dv.Create {
				if !fv.F.Role {
					register = append(register, fv)
				}
			}
			gv.RegisterFrom = conversion(e.Name+"CreateRequest", "registerRequest", register, false)
		}
	}
	filters := func(message string, skip *schema.Field) string {
		msg := file.Message(message)
		byName := messageFields(msg)
		var c strings.Builder
		c.WriteString("var p ListParams\nvar err error\nif p.Limit, p.Offset, err = grpcx.Page(req.GetLimit(), req.GetOffset()); err != nil {\nreturn ListParams{}, err\n}\nvar errs validation.Errors\n")
		for _, fv := range dv.Filters {
			if fv.F == skip {
				continue
			}
			if fd := byName[contract.ProtoName(fv.F.Name)]; fd != nil {
				fmt.Fprintf(&c, "p.%s = %s\n", fv.Name, filterValue(fv.F, "req."+fd.GoName))
			}
		}
		c.WriteString("if len(errs) > 0 {\nreturn ListParams{}, grpcx.Problem(codes.InvalidArgument, http.StatusBadRequest, \"the query string contains invalid parameters\", errs)\n}\nreturn p, nil")
		return c.String()
	}
	gv.ListFrom = filters(e.Name+"ListRequest", nil)

	addPeer := func(target *schema.Entity) *peerClient {
		pc := peerOf(target)
		if _, ok := peers[pc.Field]; !ok {
			peers[pc.Field] = pc
			gv.Peers = append(gv.Peers, pc)
			if pc.Import != gv.Import && !slices.ContainsFunc(gv.PeerImports, func(i goImport) bool { return i.Path == pc.Import }) {
				gv.PeerImports = append(gv.PeerImports, goImport{Name: pc.Alias, Path: pc.Import})
			}
		}
		return pc
	}
	for _, fv := range dv.Fields {
		f := fv.F
		if f.Ref == nil || f.Owner || f.Ref.Target == nil || !f.External(e) || (!f.InCreate() && !f.InUpdate()) {
			continue
		}
		pc := addPeer(f.Ref.Target)
		value := "in." + f.Name
		if f.Nullable {
			value = "*" + value
		}
		gv.Checks = append(gv.Checks, remoteCheck{
			Client:   pc.Field,
			Alias:    pc.Alias,
			Request:  f.Ref.Target.Name + "ExistsRequest",
			Field:    f.Name,
			JSON:     f.JSON,
			Value:    idProto(f.Ref.Target.PK(), value),
			Nullable: f.Nullable,
		})
	}
	for _, rv := range dv.Relations {
		if !dv.Routes {
			break
		}
		list, link, _ := schema.RelationPolicies(rv.Source, e)
		relOwned := owned || rv.Source.Owner() != nil
		relOwner := ""
		if rv.Owned {
			relOwner = "caller.ID, "
		}
		gr := &grpcRelation{relationView: rv}
		gr.List = grpcOp{Name: rv.Method, Request: rv.Method + "Request", Authorize: authorizeCode(list, relOwned, enabled), Owner: relOwner}
		gr.ParamsFunc = "listParams" + rv.Method
		gr.ParamsBody = filters(rv.Method+"Request", relationFK(rv))
		gr.SourceParse = idParse(rv.Source.PK(), "req.GetSourceId()", rv.Source.PathParam(), rv.SourceIDVar)
		if rv.ExternalSource {
			pc := addPeer(rv.Source)
			gr.SourceCheck = &remoteCheck{Client: pc.Field, Alias: pc.Alias, Request: rv.Source.Name + "ExistsRequest", Value: idProto(rv.Source.PK(), rv.SourceIDVar)}
		}
		if rv.Writable {
			gr.Link = grpcOp{Name: rv.LinkMethod, Request: rv.LinkMethod + "Request", Authorize: authorizeCode(link, relOwned, enabled), Owner: relOwner}
			gr.Unlink = grpcOp{Name: rv.UnlinkMethod, Request: rv.UnlinkMethod + "Request", Authorize: authorizeCode(link, relOwned, enabled), Owner: relOwner}
			gr.TargetIDExpr = idParse(pk, "req.GetId()", e.PathParam(), "id")
		}
		gv.Relations = append(gv.Relations, gr)
	}
	return gv
}

func relationFK(rv *relationView) *schema.Field {
	if rv.FK != nil {
		return rv.FK.F
	}
	return nil
}

func peerField(target *schema.Entity) string {
	return naming.LowerCamel(target.Name) + "Client"
}

type goImport struct {
	Name string
	Path string
}
