#!/usr/bin/env python

from flask import Flask, render_template
from requests import get
import json

app = Flask(__name__)

@app.route('/')
def routeIndex():
    clients = get("http://10.140.80.1:5000/api/v1/clients/all")
    clients = clients.json()
    #print(clients[0]['clientType'])
    return render_template("status.html", clients=clients)

@app.route('/config')
def routeConfig():
    configList = {}
    configs = get("http://10.140.80.1:5000/api/v1/configs/all")
    configs = configs.json()

    for config in configs['configs']:
        appConfig = get("http://10.140.80.1:5000/api/v1/configs/" + config)
        appConfig = appConfig.json()
        configList.update(appConfig)

    return render_template("config.html", configList=configList)

if __name__ == "__main__":
    app.run(debug='True', host= '0.0.0.0', port=5500)
