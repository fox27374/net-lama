#!/usr/bin/env python

from flask import Flask, render_template
from flask_wtf.csrf import CSRFProtect
from forms import configForm
import os

SECRET_KEY = os.urandom(32)
csrf = CSRFProtect()

app = Flask(__name__)
app.config['SECRET_KEY'] = SECRET_KEY
app.static_folder = 'static'
csrf.init_app(app)
#app.config['WTF_CSRF_ENABLED'] = False




@app.route('/', methods = ['GET', 'POST'])
def signup():
    form = configForm()
    if form.validate_on_submit():
        data = form.sensorPiInterface.data
        return render_template('data.html', data=data)
    #return render_template('configForm.html', form=form)
    return render_template('test.html')

@app.route("/restart/", methods=['POST'])
def restart():
    #Moving forward code
    forward_message = "Restarting..."
    return render_template('data.html', forward_message=forward_message);

app.run(debug='True', host= '0.0.0.0', port=5500)
