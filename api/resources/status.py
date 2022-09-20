from flask_restful import Resource
#from flask_jwt_extended import jwt_required

class Status(Resource):

    #@jwt_required()
    def get(self):
        # Return the current status
        return {"message": f"Service running"}, 200

