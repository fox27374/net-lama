from flask_restful import Resource
from subprocess import Popen
#from flask_jwt_extended import jwt_required

def start_scheduler():
        command = [
            "python", 
            "bin/scheduler.py"
            ]
        p = Popen(command)
        return p.pid

class Start(Resource):

    #@jwt_required()
    def get(self):
        # Return the current status
        pid = start_scheduler()
        return {"message": f"Service running with PID {pid}"}, 200
