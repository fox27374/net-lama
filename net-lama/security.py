from models.user import UserModel
from werkzeug.security import safe_str_cmp

def authenticate(userName, userPass):
    user = UserModel.findByName(userName)
    if user and safe_str_cmp(user.userPass, userPass):
        return user

def identity(payload):
    userId = payload['identity']
    print(userId)
    return UserModel.findById(userId)