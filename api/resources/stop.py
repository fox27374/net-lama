#!/usr/bin/env python

from sys import path
path.append('/home/net-lama/')

from os import kill
from os.path import exists
from signal import SIGTERM
from flask_restful import Resource
from modules.splib import writePid, getPid
#from flask_jwt_extended import jwt_required

class Stop(Resource):

    #@jwt_required()
    def get(self):
        if exists('pid'):
            pid = getPid()
            kill(pid, SIGTERM)
            writePid("")
            return {"message": f"Service with PID {pid} stopped"}, 200
        else:
            return {"message": f"Service not running"}, 200

