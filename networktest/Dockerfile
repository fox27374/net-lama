# ./Dockerfile

FROM python:3.8-alpine

# Labeling
LABEL "org.lnxnet.project"="net-lama"
LABEL "org.lnxnet.application"="networktest"

# Set variables
ENV USER=net-lama
ENV GROUP=net-lama
ENV HOME /home/$USER

# Install additional packages
RUN apk add --no-cache bind-tools iputils gcc g++ libc-dev

# Create user and group
RUN addgroup $GROUP
RUN adduser -G $GROUP -h $HOME -D $USER

# Use new user and directory
USER $USER
WORKDIR $HOME

# Copy requirements file
COPY --chown=$USER:$GROUP networktest/requirements.txt .

# Set environment to the net-lama directory
ENV VIRTUAL_ENV=$HOME/venv
RUN python3 -m venv $VIRTUAL_ENV
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

# Install dependencies:
RUN pip install --upgrade pip
RUN pip install --no-cache-dir wheel
RUN pip install --no-cache-dir -r requirements.txt

# Copy source files
COPY --chown=$USER:$GROUP networktest/clientInfo.json .
COPY --chown=$USER:$GROUP includes ./modules
COPY --chown=$USER:$GROUP api ./api
COPY --chown=$USER:$GROUP networktest/bin ./bin
COPY --chown=$USER:$GROUP networktest/app.py .

# Run the application:
CMD ["python", "app.py"]
