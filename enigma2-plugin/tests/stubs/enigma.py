class eRCInput(object):
    kmAscii = 1
    @staticmethod
    def getInstance():
        class X(object):
            def getKeyboardMode(self): return 0
            def setKeyboardMode(self, m): pass
        return X()
def getPrevAsciiCode(): return 0
