class NumericalTextInput(object):
    def __init__(self, nextFunc=None, handleTimeout=True, search=False, mapping=None):
        self.lastKey = -1
    def timeout(self): pass
    def getKey(self, k): return "x"
class NumericalHexInput(NumericalTextInput): pass
