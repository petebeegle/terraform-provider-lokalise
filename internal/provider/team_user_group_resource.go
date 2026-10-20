// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/petebeegle/lokalise-provider/internal/client"
)

var (
	_ resource.Resource                     = &TeamUserGroupResource{}
	_ resource.ResourceWithConfigure        = &TeamUserGroupResource{}
	_ resource.ResourceWithConfigValidators = &TeamUserGroupResource{}
	_ resource.ResourceWithImportState      = &TeamUserGroupResource{}
)

// NewTeamUserGroupResource returns the lokalise_team_user_group resource.
func NewTeamUserGroupResource() resource.Resource {
	return &TeamUserGroupResource{}
}

// TeamUserGroupResource manages a single team user group's own fields
// (name, role_id/admin_rights, languages). It is non-authoritative over a
// group's members and projects: those are managed by
// lokalise_team_user_group_member and lokalise_team_user_group_project
// (Task 7), and this resource's Update never touches them.
type TeamUserGroupResource struct {
	client client.API
}

// TeamUserGroupResourceModel is the lokalise_team_user_group state/plan
// model.
type TeamUserGroupResourceModel struct {
	ID          types.String `tfsdk:"id"`
	TeamID      types.Int64  `tfsdk:"team_id"`
	Name        types.String `tfsdk:"name"`
	RoleID      types.Int64  `tfsdk:"role_id"`
	AdminRights types.Set    `tfsdk:"admin_rights"`
	Languages   types.Object `tfsdk:"languages"`
	GroupID     types.Int64  `tfsdk:"group_id"`
}

// teamUserGroupLanguagesModel is the value of the `languages` attribute:
// system language ISO codes split into reference (viewable) and
// contributable (translatable) sets.
type teamUserGroupLanguagesModel struct {
	Reference     types.Set `tfsdk:"reference"`
	Contributable types.Set `tfsdk:"contributable"`
}

func (r *TeamUserGroupResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_team_user_group"
}

func (r *TeamUserGroupResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A team user group on a Lokalise team. This resource manages exactly the group's own fields (name, role_id/admin_rights, languages); it never adds or removes members or project assignments.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "`team_id:group_id`.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"team_id": schema.Int64Attribute{
				MarkdownDescription: "Lokalise team ID. Changing it forces a new resource.",
				Required:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Group name.",
				Required:            true,
			},
			"role_id": schema.Int64Attribute{
				MarkdownDescription: "Permission template ID to assign (see `lokalise_permission_template`). Conflicts with `admin_rights`; exactly one of the two must be set.",
				Optional:            true,
			},
			"admin_rights": schema.SetAttribute{
				MarkdownDescription: "Explicit set of admin rights to assign. Conflicts with `role_id`; exactly one of the two must be set.",
				ElementType:         types.StringType,
				Optional:            true,
			},
			"languages": schema.SingleNestedAttribute{
				MarkdownDescription: "Languages this group may access, as system language ISO codes.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"reference": schema.SetAttribute{
						MarkdownDescription: "Languages the group may view but not translate.",
						ElementType:         types.StringType,
						Optional:            true,
					},
					"contributable": schema.SetAttribute{
						MarkdownDescription: "Languages the group may translate.",
						ElementType:         types.StringType,
						Optional:            true,
					},
				},
			},
			"group_id": schema.Int64Attribute{
				MarkdownDescription: "Lokalise group ID.",
				Computed:            true,
			},
		},
	}
}

func (r *TeamUserGroupResource) ConfigValidators(ctx context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		resourcevalidator.ExactlyOneOf(
			path.MatchRoot("role_id"),
			path.MatchRoot("admin_rights"),
		),
	}
}

func (r *TeamUserGroupResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req.ProviderData, &resp.Diagnostics)
}

// systemLanguageMaps lists Lokalise's system languages and returns them as
// ISO->ID and ID->ISO maps, for converting the `languages` attribute's ISO
// codes to and from the IDs the API uses.
func (r *TeamUserGroupResource) systemLanguageMaps(ctx context.Context) (map[string]int64, map[int64]string, diag.Diagnostics) {
	var diags diag.Diagnostics

	langs, err := r.client.ListSystemLanguages(ctx)
	if err != nil {
		diags.AddError("Error listing Lokalise system languages", err.Error())
		return nil, nil, diags
	}

	isoToID := make(map[string]int64, len(langs))
	idToISO := make(map[int64]string, len(langs))
	for _, l := range langs {
		isoToID[l.ISO] = l.ID
		idToISO[l.ID] = l.ISO
	}
	return isoToID, idToISO, diags
}

func (r *TeamUserGroupResource) groupInputFromPlan(ctx context.Context, plan TeamUserGroupResourceModel, isoToID map[string]int64) (client.GroupInput, diag.Diagnostics) {
	var diags diag.Diagnostics

	in := client.GroupInput{Name: plan.Name.ValueString()}
	if !plan.RoleID.IsNull() {
		roleID := plan.RoleID.ValueInt64()
		in.RoleID = &roleID
	}
	if !plan.AdminRights.IsNull() && !plan.AdminRights.IsUnknown() {
		var rights []string
		diags.Append(plan.AdminRights.ElementsAs(ctx, &rights, false)...)
		if diags.HasError() {
			return in, diags
		}
		in.AdminRights = rights
	}

	langs, d := groupLanguagesFromObject(ctx, plan.Languages, isoToID)
	diags.Append(d...)
	in.Languages = langs
	return in, diags
}

func (r *TeamUserGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan TeamUserGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	teamID := plan.TeamID.ValueInt64()

	isoToID, idToISO, diags := r.systemLanguageMaps(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	in, diags := r.groupInputFromPlan(ctx, plan, isoToID)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	g, err := r.client.CreateGroup(ctx, teamID, in)
	if err != nil {
		resp.Diagnostics.AddError("Error creating Lokalise team user group", err.Error())
		return
	}

	resp.Diagnostics.Append(plan.applyGroup(ctx, teamID, g, idToISO)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *TeamUserGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state TeamUserGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	teamID := state.TeamID.ValueInt64()

	g, err := r.client.GetGroup(ctx, teamID, state.GroupID.ValueInt64())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Lokalise team user group", err.Error())
		return
	}

	_, idToISO, diags := r.systemLanguageMaps(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(state.applyGroup(ctx, teamID, g, idToISO)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *TeamUserGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state TeamUserGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	teamID := state.TeamID.ValueInt64()

	isoToID, idToISO, diags := r.systemLanguageMaps(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	in, diags := r.groupInputFromPlan(ctx, plan, isoToID)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	g, err := r.client.UpdateGroup(ctx, teamID, state.GroupID.ValueInt64(), in)
	if err != nil {
		resp.Diagnostics.AddError("Error updating Lokalise team user group", err.Error())
		return
	}

	resp.Diagnostics.Append(plan.applyGroup(ctx, teamID, g, idToISO)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *TeamUserGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state TeamUserGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteGroup(ctx, state.TeamID.ValueInt64(), state.GroupID.ValueInt64())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting Lokalise team user group", err.Error())
	}
}

func (r *TeamUserGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts, err := splitID(req.ID, 2, "team_id:group_id")
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", err.Error())
		return
	}

	teamID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("invalid team_id %q: %s", parts[0], err))
		return
	}
	groupID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("invalid group_id %q: %s", parts[1], err))
		return
	}

	g, err := r.client.GetGroup(ctx, teamID, groupID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Lokalise team user group", err.Error())
		return
	}

	_, idToISO, diags := r.systemLanguageMaps(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var model TeamUserGroupResourceModel
	resp.Diagnostics.Append(model.applyGroup(ctx, teamID, g, idToISO)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

// applyGroup copies API-owned fields from g into m and recomputes id.
//
// Whether role_id or admin_rights ends up set follows the same rule as
// lokalise_contributor (see useRoleIDOverAdminRights): whichever the user
// configured is preserved instead of being re-derived from the API response
// on every read.
func (m *TeamUserGroupResourceModel) applyGroup(ctx context.Context, teamID int64, g *client.Group, idToISO map[int64]string) diag.Diagnostics {
	var diags diag.Diagnostics

	useRoleID := useRoleIDOverAdminRights(m.RoleID.IsNull(), m.AdminRights.IsNull(), g.RoleID != nil)

	m.TeamID = types.Int64Value(teamID)
	m.Name = types.StringValue(g.Name)
	m.GroupID = types.Int64Value(g.ID)
	m.ID = types.StringValue(joinID(strconv.FormatInt(teamID, 10), strconv.FormatInt(g.ID, 10)))

	if useRoleID {
		if g.RoleID != nil {
			m.RoleID = types.Int64Value(*g.RoleID)
		} else {
			m.RoleID = types.Int64Null()
		}
		m.AdminRights = types.SetNull(types.StringType)
	} else {
		m.RoleID = types.Int64Null()
		rights, d := types.SetValueFrom(ctx, types.StringType, g.AdminRights)
		diags.Append(d...)
		m.AdminRights = rights
	}

	langs, d := groupLanguagesToObject(ctx, g.Languages, idToISO, m.Languages)
	diags.Append(d...)
	m.Languages = langs

	return diags
}

// teamUserGroupLanguagesAttrTypes is the attribute-type map of the
// `languages` object.
func teamUserGroupLanguagesAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"reference":     types.SetType{ElemType: types.StringType},
		"contributable": types.SetType{ElemType: types.StringType},
	}
}

// groupLanguagesFromObject converts the `languages` object (ISO codes) into
// client.GroupLanguages (system language IDs), using isoToID. A null or
// unknown object yields a zero GroupLanguages. An ISO code absent from
// isoToID produces an "unknown language ISO code" error.
func groupLanguagesFromObject(ctx context.Context, obj types.Object, isoToID map[string]int64) (client.GroupLanguages, diag.Diagnostics) {
	var diags diag.Diagnostics
	if obj.IsNull() || obj.IsUnknown() {
		return client.GroupLanguages{}, diags
	}

	var m teamUserGroupLanguagesModel
	diags.Append(obj.As(ctx, &m, basetypes.ObjectAsOptions{})...)
	if diags.HasError() {
		return client.GroupLanguages{}, diags
	}

	reference, d := isoSetToIDs(ctx, m.Reference, isoToID)
	diags.Append(d...)
	contributable, d2 := isoSetToIDs(ctx, m.Contributable, isoToID)
	diags.Append(d2...)
	if diags.HasError() {
		return client.GroupLanguages{}, diags
	}

	return client.GroupLanguages{Reference: reference, Contributable: contributable}, diags
}

// isoSetToIDs converts a set of ISO codes into system language IDs, erroring
// on any code not found in isoToID. A null or unknown set yields nil.
func isoSetToIDs(ctx context.Context, s types.Set, isoToID map[string]int64) ([]int64, diag.Diagnostics) {
	var diags diag.Diagnostics
	if s.IsNull() || s.IsUnknown() {
		return nil, diags
	}

	var isos []string
	diags.Append(s.ElementsAs(ctx, &isos, false)...)
	if diags.HasError() {
		return nil, diags
	}

	out := make([]int64, 0, len(isos))
	for _, iso := range isos {
		id, ok := isoToID[iso]
		if !ok {
			diags.AddError("Unknown language", fmt.Sprintf("unknown language ISO code %q", iso))
			return nil, diags
		}
		out = append(out, id)
	}
	return out, diags
}

// groupLanguagesToObject converts client.GroupLanguages (system language
// IDs) into the `languages` object (ISO codes), using idToISO. prior is the
// current plan/state value and decides how "nothing" is represented, so a
// configured form round-trips unchanged:
//
//   - the object is null when the API returns no languages and prior is
//     null (or unknown), so an omitted `languages` stays omitted;
//   - otherwise it is an object, and each set that the API returns empty
//     keeps prior's form if that was null or empty, and is null otherwise.
//
// A language ID absent from idToISO produces an "unknown language ID" error.
func groupLanguagesToObject(ctx context.Context, langs client.GroupLanguages, idToISO map[int64]string, prior types.Object) (types.Object, diag.Diagnostics) {
	var diags diag.Diagnostics
	nullObj := types.ObjectNull(teamUserGroupLanguagesAttrTypes())

	priorKnown := !prior.IsNull() && !prior.IsUnknown()
	if len(langs.Reference) == 0 && len(langs.Contributable) == 0 && !priorKnown {
		return nullObj, diags
	}

	var priorModel teamUserGroupLanguagesModel
	if priorKnown {
		diags.Append(prior.As(ctx, &priorModel, basetypes.ObjectAsOptions{})...)
		if diags.HasError() {
			return nullObj, diags
		}
	}

	reference, d := idsToISOSet(ctx, langs.Reference, idToISO, priorModel.Reference)
	diags.Append(d...)
	contributable, d2 := idsToISOSet(ctx, langs.Contributable, idToISO, priorModel.Contributable)
	diags.Append(d2...)
	if diags.HasError() {
		return nullObj, diags
	}

	obj, d3 := types.ObjectValue(teamUserGroupLanguagesAttrTypes(), map[string]attr.Value{
		"reference":     reference,
		"contributable": contributable,
	})
	diags.Append(d3...)
	return obj, diags
}

// idsToISOSet converts a slice of system language IDs into a set of ISO
// codes, erroring on any ID not found in idToISO. An empty slice yields an
// empty set if prior is a known empty set, and a null set otherwise.
func idsToISOSet(ctx context.Context, ids []int64, idToISO map[int64]string, prior types.Set) (types.Set, diag.Diagnostics) {
	var diags diag.Diagnostics
	if len(ids) == 0 {
		if !prior.IsNull() && !prior.IsUnknown() && len(prior.Elements()) == 0 {
			return types.SetValueMust(types.StringType, []attr.Value{}), diags
		}
		return types.SetNull(types.StringType), diags
	}

	isos := make([]string, 0, len(ids))
	for _, id := range ids {
		iso, ok := idToISO[id]
		if !ok {
			diags.AddError("Unknown language", fmt.Sprintf("unknown language ID %d", id))
			return types.SetNull(types.StringType), diags
		}
		isos = append(isos, iso)
	}

	set, d := types.SetValueFrom(ctx, types.StringType, isos)
	diags.Append(d...)
	return set, diags
}
