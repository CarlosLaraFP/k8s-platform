
	from fastapi import FastAPI
	app = FastAPI()

	@app.get("/")
	def read_root():
	    return {"message": "Model server is ready"}

	@app.post("/predict")
	def predict():
	    return {"prediction": "fake prediction"}
	