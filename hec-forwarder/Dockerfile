# ./Dockerfile

FROM python:3.8-alpine

# Install git
RUN apk update && apk add git

# Set some variables
ENV USER=net-lama
ENV GROUP=net-lama
ENV ID=27374
ENV HOME /home/$USER
ENV APP=hec-forwarder

# Create user and group
RUN addgroup -g $ID $GROUP
RUN adduser -u $ID -G $GROUP -h $HOME -D $USER

# Use new user and directory
USER $USER
WORKDIR $HOME

# Clone GIT repository
RUN git clone https://github.com/fox27374/net-lama.git

# Set new workdir to the correct folder
WORKDIR $HOME/net-lama/$APP

# Set environment to the net-lama directory
ENV VIRTUAL_ENV=$HOME/net-lama/$APP/venv
RUN python3 -m venv $VIRTUAL_ENV
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

# Install dependencies:
RUN pip install --no-cache-dir -r requirements.txt

# Run the application:
CMD ["python", "app.py"]
