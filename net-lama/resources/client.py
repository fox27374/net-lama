from flask_restful import Resource, reqparse
from flask_jwt import jwt_required
from models.client import ClientModel

class Client(Resource):
    parser = reqparse.RequestParser()
    parser.add_argument(
        'clientId', 
        type=str,
        required=False,
        help="Cannot be left blank")
    parser.add_argument(
        'clientType', 
        type=str,
        required=True,
        help="Cannot be left blank")

    def get(self, clientId=None):
        # Return a list of all Clients of no clientId is given
        if clientId == None:
            return {'Clients': [org.json() for org in ClientModel.query.all()]}

        org = ClientModel.findById(clientId)
        if org:
            return org.json()
        return {"message": f"Client {clientId} not found"}, 404

    def post(self, clientId=None):
        if clientId:
            return {"message": "clientId not allowed in the request"}, 400

        data = Client.parser.parse_args()
        print(f"Client data: {data}") 
        org = ClientModel(**data)

        try:
            org.save()
        except:
            return {"message": "An error occured inserting the client"}, 500 

        return org.json(), 201

    def delete(self, clientId=None):
        if clientId == None:
            return {"message": "clientId has to be send in the request"}, 400

        org = ClientModel.findById(clientId)
        if org:
            org.delete()
            return {"message": f"Client {clientId} deleted"}

        return {"message": f"Client {clientId} not found"}, 404

    def put(self, clientId=None):
        data = Client.parser.parse_args()
        org = ClientModel.findById(clientId)       
        
        # Create a new Client
        if org is None and clientId is None:
            org = ClientModel(**data)
        elif org is None and clientId:
            return {"message": f"Client with clientId {clientId} not found"}, 400
        elif org and clientId is None:
            return {"message": "clientId has to be statet in order to update the Client"}, 400
        else:
            org.clientType = data['clientType']

        org.save()
         
        return org.json()
        
