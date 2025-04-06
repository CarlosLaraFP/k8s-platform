package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/crossplane/function-docker-build/input/v1beta1"
	"github.com/crossplane/function-sdk-go/errors"
	"github.com/crossplane/function-sdk-go/logging"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/request"
	"github.com/crossplane/function-sdk-go/response"
)

// Function returns whatever response you ask it to.
type Function struct {
	fnv1.UnimplementedFunctionRunnerServiceServer

	log logging.Logger
}

// RunFunction runs the Function.
func (f *Function) RunFunction(_ context.Context, req *fnv1.RunFunctionRequest) (*fnv1.RunFunctionResponse, error) {
	f.log.Info("Running function", "tag", req.GetMeta().GetTag())

	rsp := response.To(req, response.DefaultTTL)

	in := &v1beta1.Input{}
	if err := request.GetInput(req, in); err != nil {
		// You can set a custom status condition on the claim. This allows you to
		// communicate with the user. See the link below for status condition
		// guidance.
		// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties
		response.ConditionFalse(rsp, "FunctionSuccess", "InternalError").
			WithMessage("Something went wrong.").
			TargetCompositeAndClaim()

		// You can emit an event regarding the claim. This allows you to communicate
		// with the user. Note that events should be used sparingly and are subject
		// to throttling; see the issue below for more information.
		// https://github.com/crossplane/crossplane/issues/5802
		response.Warning(rsp, errors.New("something went wrong")).
			TargetCompositeAndClaim()

		response.Fatal(rsp, errors.Wrapf(err, "cannot get Function input from %T", req))
		return rsp, nil
	}

	// TODO: Add your Function logic here!
	response.Normalf(rsp, "I was run with input %q!", in.Example)
	f.log.Info("I was run!", "input", in.Example)

	// You can set a custom status condition on the claim. This allows you to
	// communicate with the user. See the link below for status condition
	// guidance.
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties
	response.ConditionTrue(rsp, "FunctionSuccess", "Success").
		TargetCompositeAndClaim()

	//////////////
	// Build Docker image
	err := buildAndLoadDockerImage(req.Input.GetFields()["requirementsPath"].GetStringValue(), req.Input.GetFields()["name"].GetStringValue())
	if err != nil {
		return nil, fmt.Errorf("failed to build and load docker image: %w", err)
	}

	// Patch the XR with the image field
	patch := map[string]interface{}{
		"spec": map[string]interface{}{
			"image": req.Input.GetFields()["name"].GetStringValue() + ":latest", // dynamically built image
		},
	}

	_, err = json.Marshal(patch)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal patch: %w", err)
	}
	//////////////

	return rsp, nil
}

func buildAndLoadDockerImage(requirementsPath string, name string) error {
	// Assume requirementsPath is a file path mounted to this pod
	dockerfileContent := `
	FROM python:3.11-slim
	WORKDIR /app
	COPY requirements.txt .
	RUN pip install -r requirements.txt
	COPY app.py .
	CMD ["uvicorn", "app:app", "--host", "0.0.0.0", "--port", "5000"]
	`
	err := os.WriteFile("Dockerfile", []byte(dockerfileContent), 0644)
	if err != nil {
		return fmt.Errorf("failed to write Dockerfile: %w", err)
	}

	// Create app.py dynamically
	appPyContent := `
	from fastapi import FastAPI
	app = FastAPI()

	@app.get("/")
	def read_root():
	    return {"message": "Model server is ready"}

	@app.post("/predict")
	def predict():
	    return {"prediction": "fake prediction"}
	`
	err = os.WriteFile("app.py", []byte(appPyContent), 0644)
	if err != nil {
		return fmt.Errorf("failed to write app.py: %w", err)
	}

	// Copy the requirements.txt to local directory
	err = exec.Command("cp", requirementsPath, "./requirements.txt").Run()
	if err != nil {
		return fmt.Errorf("failed to copy requirements.txt: %w", err)
	}

	// Docker build
	err = exec.Command("docker", "build", "-t", name+":latest", ".").Run()
	if err != nil {
		return fmt.Errorf("docker build failed: %w", err)
	}

	// Load into KinD
	err = exec.Command("kind", "load", "docker-image", name+":latest").Run()
	if err != nil {
		return fmt.Errorf("kind load docker-image failed: %w", err)
	}

	return nil
}
