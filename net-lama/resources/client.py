from flask_restful import Resource, reqparse
from flask_jwt_extended import jwt_required
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

    @jwt_required()
    def get(self, clientId=None):
        # Return a list of all Clients of no clientId is given
        if clientId == None:
            return {'Clients': [client.json() for client in ClientModel.query.all()]}

        client = ClientModel.findById(clientId)
        if client:
            return client.json()
        return {"message": f"Client {clientId} not found"}, 404

    @jwt_required()
    def post(self, clientId=None):
        if clientId:
            return {"message": "clientId not allowed in the request"}, 400

        data = Client.parser.parse_args()
        client = ClientModel(**data)

        try:
            client.save()
        except:
            return {"message": "An error occured inserting the client"}, 500 

        return client.json(), 201

    @jwt_required()
    def delete(self, clientId=None):
        if clientId == None:
            return {"message": "clientId has to be send in the request"}, 400

        client = ClientModel.findById(clientId)
        if client:
            client.delete()
            return {"message": f"Client {clientId} deleted"}

        return {"message": f"Client {clientId} not found"}, 404

    @jwt_required()
    def put(self, clientId=None):
        data = Client.parser.parse_args()
        client = ClientModel.findById(clientId)       
        
        # Create a new Client
        if client is None and clientId is None:
            client = ClientModel(**data)
        elif client is None and clientId:
            return {"message": f"Client with clientId {clientId} not found"}, 400
        elif client and clientId is None:
            return {"message": "clientId has to be statet in order to update the Client"}, 400
        else:
            client.clientType = data['clientType']

        client.save()
         
        return client.json()
        
