// Copyright (c) real-cis <info@real-cis.com>
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
)

// immutableAfterCreate rejects any change to an attribute once the resource exists, instead of
// planning a replacement that would destroy it. A null prior value is accepted and written to
// state as-is: that only happens after an import, for attributes the API can't read back.
type immutableAfterCreate struct {
	resourceName string
}

var (
	_ planmodifier.String = immutableAfterCreate{}
	_ planmodifier.Int64  = immutableAfterCreate{}
	_ planmodifier.Bool   = immutableAfterCreate{}
	_ planmodifier.List   = immutableAfterCreate{}
)

func (m immutableAfterCreate) Description(ctx context.Context) string {
	return fmt.Sprintf("Can't be changed once the %s exists.", m.resourceName)
}

func (m immutableAfterCreate) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m immutableAfterCreate) check(state tfsdk.State, plan tfsdk.Plan, attrPath path.Path, stateValue, planValue attr.Value, diags *diag.Diagnostics) {
	if state.Raw.IsNull() || plan.Raw.IsNull() {
		return
	}
	if stateValue.IsNull() || planValue.IsUnknown() || planValue.Equal(stateValue) {
		return
	}

	diags.AddAttributeError(attrPath, fmt.Sprintf("Can't change %s of an existing %s", attrPath, m.resourceName), "")
}

func (m immutableAfterCreate) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	m.check(req.State, req.Plan, req.Path, req.StateValue, req.PlanValue, &resp.Diagnostics)
}

func (m immutableAfterCreate) PlanModifyInt64(ctx context.Context, req planmodifier.Int64Request, resp *planmodifier.Int64Response) {
	m.check(req.State, req.Plan, req.Path, req.StateValue, req.PlanValue, &resp.Diagnostics)
}

func (m immutableAfterCreate) PlanModifyBool(ctx context.Context, req planmodifier.BoolRequest, resp *planmodifier.BoolResponse) {
	m.check(req.State, req.Plan, req.Path, req.StateValue, req.PlanValue, &resp.Diagnostics)
}

func (m immutableAfterCreate) PlanModifyList(ctx context.Context, req planmodifier.ListRequest, resp *planmodifier.ListResponse) {
	m.check(req.State, req.Plan, req.Path, req.StateValue, req.PlanValue, &resp.Diagnostics)
}
