from models.user import UserModel
from werkzeug.security import safe_str_cmp

def authenticate(username, password):
    user = UserModel.findByName(username)
    print(user)
    if user and safe_str_cmp(user.password, password):
        return user

def identity(payload):
    userId = payload['identity']
    return UserModel.findById(userId)