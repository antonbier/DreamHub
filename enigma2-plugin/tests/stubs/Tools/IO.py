# same behaviour as the box's Tools/IO.saveFile (tmp file + rename), returns False on failure
import os, tempfile
def saveFile(filename, data, mode=0o644):
    tmp = None
    try:
        f = tempfile.NamedTemporaryFile(prefix='.%s.' % os.path.basename(filename), dir=os.path.dirname(filename), delete=False)
        tmp = f.name
        f.write(data.encode("utf-8") if not isinstance(data, bytes) else data)
        f.flush(); os.fsync(f.fileno()); os.fchmod(f.fileno(), mode); f.close()
        os.rename(tmp, filename)
    except Exception as e:
        print('saveFile: failed', e)
        return False
    return True
