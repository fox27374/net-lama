from models.user import UserModel
from flask_restful import Resource, reqparse

class User(Resource):
    parser = reqparse.RequestParser()
    parser.add_argument(
        'userName', 
        type=str,
        required=False,
        help="Cannot be left blank")
    parser.add_argument(
        'userPass', 
        type=str,
        required=True,
        help="Cannot be left blank")

    def get(self, userName=None):
        # Return a list of all user if no userName is given
        if userName == None:
            return {'User': [user.json() for user in UserModel.query.all()]}

        user = UserModel.findByName(userName)
        if user:
            return user.json()
        return {"message": f"User {userName} not found"}, 404

    def post(self):
        data = User.parser.parse_args()

        if UserModel.findByName(data['userName']):
            return {"message": f"User {data['userName']} already exists"}, 400

        user = UserModel(**data)
        user.save()

        return {"message": f"User {data['userName']} created successfully"}, 201

    def delete(self, userName=None):
        if userName == None:
            return {"message": "userName has to be send in the request"}, 400

        user = UserModel.findByName(userName)
        if user:
            user.delete()
            return {"message": f"User {userName} deleted"}

        return {"message": f"User {userName} not found"}, 404

    def put(self, userName=None):
        data = User.parser.parse_args()
        user = UserModel.findByName(userName)       
        
        # Create a new Client
        if user is None and userName is None:
            user = UserModel(**data)
        elif user is None and userName:
            return {"message": f"User {userName} not found"}, 400
        elif user and userName is None:
            return {"message": "userName has to be statet in order to update the user"}, 400
        else:
            user.userPass = data['userPass']

        user.save()
         
        return user.json()