#!/usr/bin/env python

from flask import Flask, render_template
from requests import get
import json

app = Flask(__name__)

@app.route('/')
def index():
    response = get("http://10.140.80.1:5000/api/v1/configs/SpeedTest")
    data = response.json()
    return render_template("rest.html", data=data)

if __name__ == "__main__":
    app.run(debug='True', host= '0.0.0.0', port=5500)
