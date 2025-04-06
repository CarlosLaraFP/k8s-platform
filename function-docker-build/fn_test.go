package main

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/crossplane/function-sdk-go/logging"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/resource"
)

func TestRunFunction(t *testing.T) {

	type args struct {
		ctx context.Context
		req *fnv1.RunFunctionRequest
	}
	type want struct {
		rsp *fnv1.RunFunctionResponse
		err error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"ResponseIsReturned": {
			reason: "The Function should run successfully",
			args: args{
				req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{
							Resource: resource.MustStructJSON(`{
							"apiVersion": "platform.example.org/v1alpha1",
							"kind": "ModelDeployment",
							"metadata": {
								"name": "my-model-deployment",
								"namespace": "charles123"
							},
							"spec": {
								"userName": "charles123",
								"requirementsPath": "requirements.txt"
							}
						}`),
						},
					},
				},
			},
			want: want{
				rsp: &fnv1.RunFunctionResponse{
					Conditions: []*fnv1.Condition{
						{
							Type:   "FunctionSuccess",
							Status: fnv1.Status_STATUS_CONDITION_TRUE,
							Reason: "Success",
							Target: fnv1.Target_TARGET_COMPOSITE_AND_CLAIM.Enum(),
						},
					},
					Meta: &fnv1.ResponseMeta{
						Ttl: durationpb.New(60 * time.Second),
					},
				},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := &Function{log: logging.NewNopLogger()}
			rsp, err := f.RunFunction(tc.args.ctx, tc.args.req)

			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("%s\nf.RunFunction(...): -want err, +got err:\n%s", tc.reason, diff)
			}

			// Check conditions
			if diff := cmp.Diff(tc.want.rsp.Conditions, rsp.Conditions, protocmp.Transform()); diff != "" {
				t.Errorf("%s\nf.RunFunction(...): -want conditions, +got conditions:\n%s", tc.reason, diff)
			}

			// Check that desired resources exist
			if len(rsp.Desired.Resources) != 1 {
				t.Errorf("%s\nexpected 1 desired resource, got %d", tc.reason, len(rsp.Desired.Resources))
			}

			for _, desired := range rsp.Desired.Resources {
				fields := desired.Resource.Fields

				// Top-level fields
				if fields["apiVersion"].GetStringValue() != "platform.example.org/v1alpha1" {
					t.Errorf("expected apiVersion platform.example.org/v1alpha1, got %s", fields["apiVersion"].GetStringValue())
				}

				if fields["kind"].GetStringValue() != "ModelDeployment" {
					t.Errorf("expected kind ModelDeployment, got %s", fields["kind"].GetStringValue())
				}

				// Access nested fields
				specFields := fields["spec"].GetStructValue().Fields

				if specFields["userName"].GetStringValue() != "charles123" {
					t.Errorf("expected userName charles123, got %s", specFields["userName"].GetStringValue())
				}

				if specFields["requirementsPath"].GetStringValue() != "requirements.txt" {
					t.Errorf("expected requirementsPath requirements.txt, got %s", specFields["requirementsPath"].GetStringValue())
				}

				if specFields["image"].GetStringValue() == "" {
					t.Errorf("expected spec.image to be set, but it was empty")
				}
			}

		})
	}

}
