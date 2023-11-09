import deprecated

def hello_world():
  return "Hello, world!"

class Dog(object):
  def __init__(self) -> None:
    pass
  
  def speak(self):
    return "Woof!"

@deprecated.deprecated("This method is bad!")
def bad_method():
  return "I'm a bad method!"