from models.user import UserModel
from flask_restful import Resource, reqparse

class User(Resource):
    parser = reqparse.RequestParser()
    parser.add_argument(
        'username', 
        type=str,
        required=False,
        help="username be left blank")
    parser.add_argument(
        'password', 
        type=str,
        required=True,
        help="password be left blank")

    def get(self, username=None):
        # Return a list of all user if no username is given
        if username == None:
            return {'User': [user.json() for user in UserModel.query.all()]}

        user = UserModel.findByName(username)
        if user:
            return user.json()
        return {"message": f"User {username} not found"}, 404

    def post(self):
        data = User.parser.parse_args()

        if UserModel.findByName(data['username']):
            return {"message": f"User {data['username']} already exists"}, 400

        user = UserModel(**data)
        user.save()

        return {"message": f"User {data['username']} created successfully"}, 201

    def delete(self, username=None):
        if username == None:
            return {"message": "username has to be send in the request"}, 400

        user = UserModel.findByName(username)
        if user:
            user.delete()
            return {"message": f"User {username} deleted"}

        return {"message": f"User {username} not found"}, 404

    def put(self, username=None):
        data = User.parser.parse_args()
        user = UserModel.findByName(username)       
        
        # Create a new user
        if user is None and username is None:
            user = UserModel(**data)
        elif user is None and username:
            return {"message": f"User {username} not found"}, 400
        elif user and username is None:
            return {"message": "username has to be statet in order to update the user"}, 400
        else:
            user.password = data['password']

        user.save()
         
        return user.json()