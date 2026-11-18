package plugin

import (
	"context"
	"time"

	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/rshade/finfocus-plugin-cloudzero/internal/mapping"
)

// BatchCost answers actual-cost batches. Other query types get a per-resource
// Unimplemented error. Results stay in request order.
func (p *CloudZeroPlugin) BatchCost(ctx context.Context, req *pbc.BatchCostRequest) (*pbc.BatchCostResponse, error) {
	if req == nil || len(req.GetResources()) == 0 {
		return emptyBatch(), nil
	}
	if req.GetQueryType() != pbc.CostQueryType_COST_QUERY_TYPE_ACTUAL {
		return unimplementedBatch(req.GetResources()), nil
	}
	start, end, err := window(req.GetStart(), req.GetEnd())
	if err != nil {
		return nil, err
	}
	results := make([]*pbc.ResourceCostResult, len(req.GetResources()))
	if err = p.ready(); err != nil {
		for i, resource := range req.GetResources() {
			results[i] = resourceError(resource, err)
		}
		return &pbc.BatchCostResponse{Results: results, MaxBatchSize: defaultBatchSize}, nil
	}
	grouped := map[string][]batchMember{}
	for i, resource := range req.GetResources() {
		filter, filterErr := p.mapper.Filter(resource, nil, "", "")
		if filterErr != nil {
			results[i] = resourceError(resource, status.Error(codes.InvalidArgument, filterErr.Error()))
			continue
		}
		key := filterKey(filter.Shared)
		grouped[key] = append(
			grouped[key],
			batchMember{index: i, resource: resource, filter: filter},
		)
	}
	for _, members := range grouped {
		p.fillGroup(ctx, results, members, start, end)
	}
	return &pbc.BatchCostResponse{Results: results, MaxBatchSize: defaultBatchSize}, nil
}

type batchMember struct {
	index    int
	resource *pbc.ResourceDescriptor
	filter   mapping.Filter
}

func (p *CloudZeroPlugin) fillGroup(
	ctx context.Context, results []*pbc.ResourceCostResult, members []batchMember, start, end time.Time,
) {
	rows, err := p.costsFor(ctx, start, end, members[0].filter.Shared)
	for _, member := range members {
		if err != nil {
			results[member.index] = resourceError(member.resource, statusFrom(err))
			continue
		}
		data := &pbc.ActualCostData{Results: p.rowsFor(rows, member.filter, start, end)}
		if len(data.GetResults()) == 0 {
			data.FallbackHint = pbc.FallbackHint_FALLBACK_HINT_RECOMMENDED
		}
		results[member.index] = &pbc.ResourceCostResult{
			Resource: member.resource,
			Result: &pbc.ResourceCostResult_CostData{CostData: &pbc.CostData{
				Data: &pbc.CostData_ActualCost{ActualCost: data},
			}},
		}
	}
}

func emptyBatch() *pbc.BatchCostResponse {
	return &pbc.BatchCostResponse{Results: []*pbc.ResourceCostResult{}, MaxBatchSize: defaultBatchSize}
}

func unimplementedBatch(resources []*pbc.ResourceDescriptor) *pbc.BatchCostResponse {
	results := make([]*pbc.ResourceCostResult, len(resources))
	err := status.Error(codes.Unimplemented, "CloudZero provides actual cost only")
	for i, resource := range resources {
		results[i] = resourceError(resource, err)
	}
	return &pbc.BatchCostResponse{Results: results, MaxBatchSize: defaultBatchSize}
}

func resourceError(resource *pbc.ResourceDescriptor, err error) *pbc.ResourceCostResult {
	st, _ := status.FromError(err)
	return &pbc.ResourceCostResult{
		Resource: resource,
		Result: &pbc.ResourceCostResult_Error{Error: &pbc.ResourceError{
			Code:    int32(st.Code()), //nolint:gosec // gRPC codes fit in int32
			Message: st.Message(),
		}},
	}
}
