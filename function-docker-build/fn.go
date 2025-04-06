package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"

	"function-docker-build/input/v1beta1"

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
// Crossplane doesn’t validate function input. It’s a good idea for a function to validate its own input.
func (f *Function) RunFunction(_ context.Context, req *fnv1.RunFunctionRequest) (*fnv1.RunFunctionResponse, error) {
	f.log.Info("Running function", "tag", req.GetMeta().GetTag())
	f.log.Info("Request", "input", req.String())

	rsp := response.To(req, response.DefaultTTL)

	xr, err := request.GetObservedCompositeResource(req)
	if err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot get observed composite resource from %T", req))
		return rsp, nil
	}

	un, err := xr.Resource.GetString("spec.userName")
	if err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot read spec.userName field of %s", xr.Resource.GetKind()))
		return rsp, nil
	}

	rp, err := xr.Resource.GetString("spec.requirementsPath")
	if err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot read spec.requirementsPath field of %s", xr.Resource.GetKind()))
		return rsp, nil
	}

	in := &v1beta1.ModelDeployment{}
	if err := request.GetInput(req, in); err != nil {
		// You can set a custom status condition on the claim. This allows you to
		// communicate with the user. See the link below for status condition guidance.
		// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties
		response.ConditionFalse(rsp, "FunctionSuccess", "InternalError").
			WithMessage("Something went wrong.").
			TargetCompositeAndClaim()

		// You can emit an event regarding the claim. This allows you to communicate
		// with the user. Note that events should be used sparingly and are subject
		// to throttling; see the issue below for more information.
		// https://github.com/crossplane/crossplane/issues/5802
		response.Warning(rsp, errors.New("something went wrong")).TargetCompositeAndClaim()
		response.Fatal(rsp, errors.Wrapf(err, "cannot get Function input from %T", req))
		return rsp, nil
	}

	f.log.Info("ModelDeployment XR claim successfully received", "UserName", in.Spec.UserName)

	// Build Docker image
	image, err := buildAndLoadDockerImage(un, rp)
	if err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "failed to build and load docker image"))
		return rsp, nil
	}

	// Patch the XR with the image field
	patch := map[string]any{
		"spec": map[string]any{
			"image": image, // dynamically built image
		},
	}

	_, err = json.Marshal(patch)
	if err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "failed to marshal patch"))
		return rsp, nil
	}

	response.Normalf(rsp, "Function ran with inputs: %s and %s", un, rp)
	// You can set a custom status condition on the claim. This allows you to
	// communicate with the user. See the link below for status condition guidance.
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties
	response.ConditionTrue(rsp, "FunctionSuccess", "Success").TargetCompositeAndClaim()

	return rsp, nil
}

func buildAndLoadDockerImage(userName, requirementsPath string) (string, error) {
	if userName == "" {
		return "", fmt.Errorf("empty userName")
	}
	image := fmt.Sprintf("%s-%s:latest", userName, fmt.Sprint(time.Now().UnixNano()))
	// TODO: get user's requirements.txt file
	dockerfileContent := fmt.Sprintf(`
	FROM python:3.13-slim
	WORKDIR /app
	COPY requirements.txt .
	RUN pip install -r %s
	COPY app.py .
	CMD ["uvicorn", "app:app", "--host", "0.0.0.0", "--port", "5000"]
	`, requirementsPath)
	err := os.WriteFile("Dockerfile", []byte(dockerfileContent), 0644)
	if err != nil {
		return image, fmt.Errorf("failed to write Dockerfile: %w", err)
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
		return "", fmt.Errorf("failed to write app.py: %w", err)
	}

	// Docker build
	cmd := exec.Command("docker", "build", "-t", image, ".")
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err = cmd.Run()
	if err != nil {
		return "", fmt.Errorf("docker build failed: %w\nstdout: %s\nstderr: %s", err, outBuf.String(), errBuf.String())
	}

	// Load into KinD
	cmd = exec.Command("kind", "load", "docker-image", image, "--name", "k8s-platform")
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err = cmd.Run()
	if err != nil {
		return "", fmt.Errorf("kind load docker-image failed: %w\nstdout: %s\nstderr: %s", err, outBuf.String(), errBuf.String())
	}

	return image, nil
}
