package main

import (
	"bytes"
	"context"
	"fmt"
	v1 "function-docker-build/input/v1beta1"
	"os"
	"os/exec"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/crossplane/function-sdk-go/errors"
	"github.com/crossplane/function-sdk-go/logging"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/request"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
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

	// Create a response to the request. This copies the desired state and
	// pipeline context from the request to the response.
	rsp := response.To(req, response.DefaultTTL)

	xr, err := request.GetObservedCompositeResource(req)
	if err != nil {
		// You can set a custom status condition on the claim. This
		// allows you to communicate with the user.
		response.ConditionFalse(rsp, "FunctionSuccess", "InternalError").
			WithMessage("Something went wrong at GetObservedCompositeResource").
			TargetCompositeAndClaim()

		// You can emit an event regarding the claim. This allows you to
		// communicate with the user. Note that events should be used
		// sparingly and are subject to throttling
		response.Warning(rsp, errors.New("something went wrong")).
			TargetCompositeAndClaim()

		// If the function can't read the XR, the request is malformed. This
		// should never happen. The function returns a fatal result. This tells
		// Crossplane to stop running functions and return an error.
		response.Fatal(rsp, errors.Wrapf(err, "cannot get observed composite resource from %T", req))
		return rsp, nil
	}

	// Create an updated logger with useful information about the XR.
	log := f.log.WithValues(
		"xr-version", xr.Resource.GetAPIVersion(),
		"xr-kind", xr.Resource.GetKind(),
		"xr-name", xr.Resource.GetName(),
	)

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

	/// Get all desired composed resources from the request. The function will
	// update this map of resources, then save it. This get, update, set pattern
	// ensures the function keeps any resources added by other functions.
	desired, err := request.GetDesiredComposedResources(req)
	if err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot get desired resources from %T", req))
		return rsp, nil
	}

	// Add v1beta1 types to the composed resource scheme.
	// composed.From uses this to automatically set apiVersion and kind.
	_ = v1.AddToScheme(composed.Scheme)

	image, err := buildAndLoadDockerImage(un, rp)
	if err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "failed to build and load docker image"))
		return rsp, nil
	}

	b := &v1.ModelDeployment{
		ObjectMeta: metav1.ObjectMeta{
			// Set the external name annotation to the desired bucket name.
			// This controls what the bucket will be named in AWS.
			Annotations: map[string]string{
				"crossplane.io/patchedAt": time.Now().Local().Format(time.RFC3339),
			},
		},
		Spec: v1.ModelDeploymentSpec{
			UserName:         un,
			RequirementsPath: rp,
			Image:            image,
		},
	}

	// Convert the XR to the unstructured resource data format the SDK uses to store desired composed resources.
	cd, err := composed.From(b)
	if err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot convert %T to %T", b, &composed.Unstructured{}))
		return rsp, nil
	}

	// Add the XR to the map of desired composed resources. It's
	// important that the function adds the same XR every time it's
	// called. It's also important that the XR is added with the same
	// resource.Name every time it's called.
	desired[resource.Name(image)] = &resource.DesiredComposed{Resource: cd}

	// Finally, save the updated desired composed resources to the response.
	if err := response.SetDesiredComposedResources(rsp, desired); err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot set desired composed resources in %T", rsp))
		return rsp, nil
	}

	// Log what the function did. This will only appear in the function's pod
	// logs. A function can use response.Normal and response.Warning to emit
	// Kubernetes events associated with the XR it's operating on.
	log.Info("Patched image name", "image", image)

	// You can set a custom status condition on the claim. This allows you
	// to communicate with the user.
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
	COPY main.py .
	CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8000"]
	`, requirementsPath)
	err := os.WriteFile("tempDockerfile", []byte(dockerfileContent), 0644)
	if err != nil {
		return image, fmt.Errorf("failed to write temp Dockerfile: %w", err)
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
	err = os.WriteFile("main.py", []byte(appPyContent), 0644)
	if err != nil {
		return "", fmt.Errorf("failed to write main.py: %w", err)
	}

	// Docker build
	cmd := exec.Command("docker", "build", "-f", "tempDockerfile", "-t", image, ".")
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
