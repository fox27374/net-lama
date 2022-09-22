#!/usr/bin/env python
from sys import path
path.append('../includes/')

from flask import Flask
from flask_restful import Api
from subprocess import Popen
#from flask_jwt_extended import JWTManager
#from flask_marshmallow import Marshmallow
from resources.status import Status
from resources.start import Start
from resources.stop import Stop
from resources.update import Update

apiBaseUrl = '/api/v1/'
hostIp = '0.0.0.0'
port = 5001
debug = True

app = Flask(__name__)
app.config['PROPAGATE_EXCEPTIONS'] = True
api = Api(app)
  
#jwt = JWTManager(app)

api.add_resource(Status,
        f"{apiBaseUrl}/status"
    )
api.add_resource(Start,
        f"{apiBaseUrl}/start"
    )
api.add_resource(Stop,
        f"{apiBaseUrl}/stop"
    )
api.add_resource(Update,
        f"{apiBaseUrl}/update"
    )

def start_scheduler():
    command = [
        "python", 
        "scheduler.py"
        ]
    p = Popen(command)

    return p.pid

if __name__ == '__main__':
    scheduler_pid = start_scheduler()
    app.run(host=hostIp, port=port, debug=debug)
