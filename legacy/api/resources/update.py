#!/usr/bin/env python

from sys import path
path.append('/home/net-lama/')

from flask_restful import Resource
#from flask_jwt_extended import jwt_required

class Update(Resource):

    #@jwt_required()
    def get(self):
        # Return the current status
        return {"message": f"Service running"}, 200

