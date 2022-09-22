from models.user import UserModel
from hmac import compare_digest
from flask_jwt_extended import jwt_required, create_access_token, create_refresh_token
from flask_restful import Resource, reqparse

_userParser = reqparse.RequestParser()
_userParser.add_argument('username',
                          type=str,
                          required=True,
                          help="This field cannot be blank."
                          )
_userParser.add_argument('password',
                          type=str,
                          required=True,
                          help="This field cannot be blank."
                          )

class User(Resource):
    @jwt_required()
    def get(self, userName=None):
        # Return a list of all user if no username is given
        if userName == None:
            return {'User': [user.json() for user in UserModel.query.all()]}

        user = UserModel.findByName(userName)
        if user:
            return user.json()
        return {"message": f"User {userName} not found"}, 404

    #@jwt_required()
    def post(self):
        data = _userParser.parse_args()

        if UserModel.findByName(data['username']):
            return {"message": f"User {data['username']} already exists"}, 400

        user = UserModel(**data)
        user.save()

        return {"message": f"User {data['username']} created successfully"}, 201

    @jwt_required()
    def delete(self, userName=None):
        if userName == None:
            return {"message": "username has to be send in the request"}, 400

        user = UserModel.findByName(userName)
        if user:
            user.delete()
            return {"message": f"User {userName} deleted"}

        return {"message": f"User {userName} not found"}, 404

    @jwt_required()
    def put(self, userName=None):
        data = _userParser.parse_args()
        user = UserModel.findByName(userName)       
        
        # Create a new user
        if user is None and userName is None:
            user = UserModel(**data)
        elif user is None and userName:
            return {"message": f"User {userName} not found"}, 400
        elif user and userName is None:
            return {"message": "username has to be statet in order to update the user"}, 400
        else:
            user.password = data['password']

        user.save()
         
        return user.json()

class UserLogin(Resource):
    def post(self):
        data = _userParser.parse_args()

        user = UserModel.findByName(data['username'])

        # this is what the `authenticate()` function did in security.py
        if user and compare_digest(user.password, data['password']):
            # identity= is what the identity() function did in security.py—now stored in the JWT
            accessToken = create_access_token(identity=user.id, fresh=True) 
            refreshToken = create_refresh_token(user.id)
            return {
                'access_token': accessToken,
                'refresh_token': refreshToken
            }, 200

        return {"message": "Invalid Credentials!"}, 401