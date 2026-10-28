#!/usr/bin/env python3
"""Validate this round's editable contours and menu apertures.

Requires Pillow 13.0.0 and svgpathtools 1.7.2. Run after render_previews.py.
"""
from pathlib import Path
from math import acos, degrees
import xml.etree.ElementTree as ET
from svgpathtools import parse_path
from PIL import Image
from collections import deque
R=Path(__file__).resolve().parent
paths=joins=0;maximum=0
for f in sorted(R.glob('*/*.svg')):
    root=ET.fromstring(f.read_text())
    assert root.attrib['viewBox']=='0 0 128 128'
    for node in root.iter():
        assert node.tag.rsplit('}',1)[-1] not in ('image','text','filter','use','script','foreignObject')
        if node.tag.endswith('path'):
            paths+=1
            for sub in parse_path(node.attrib['d']).continuous_subpaths():
                assert sub.isclosed(),(f,node.attrib['id'])
                segs=[s for s in sub if s.length()>1e-8]
                for a,b in zip(segs,segs[1:]+segs[:1]):
                    u,v=a.derivative(1),b.derivative(0)
                    assert abs(u)*abs(v)>0,(f,node.attrib['id'])
                    angle=degrees(acos(max(-1,min(1,(u.conjugate()*v).real/(abs(u)*abs(v))))))
                    maximum=max(maximum,angle);joins+=1
                    assert angle<.1,(f,node.attrib['id'],a.end,angle)
print(f'GEOMETRY PASS: {paths} path objects, {joins} tangent joins, max deviation {maximum:.6f} degrees.')

def regions(points, diagonal=False):
    result=[]
    while points:
        start=points.pop();q=deque([start]);group={start}
        while q:
            x,y=q.popleft()
            for dx,dy in (((-1,0),(1,0),(0,-1),(0,1),(-1,-1),(-1,1),(1,-1),(1,1)) if diagonal else ((-1,0),(1,0),(0,-1),(0,1))):
                p=(x+dx,y+dy)
                if p in points:points.remove(p);q.append(p);group.add(p)
        result.append(group)
    return result
for name in ('a-box-cat','b-side-eye','c-coil-cat','d-sitting-cat'):
    for scale in (1,2):
        im=Image.open(R/name/'previews'/f'template-light-transparent-{scale}x.png')
        a=im.getchannel('A');n=im.width
        solid={(x,y) for y in range(n) for x in range(n) if a.getpixel((x,y))>=128}
        empty={(x,y) for y in range(n) for x in range(n) if a.getpixel((x,y))<128}
        # Eight-connected ink and four-connected empty space preserve diagonal edges.
        components=regions(solid, diagonal=True)
        holes=[s for s in regions(empty) if not any(x in (0,n-1) or y in (0,n-1) for x,y in s)]
        print(name,scale,'x:',len(components),'components;',len(holes),'holes; hole areas',list(map(len,holes)))
        assert len(components)==1,(name,scale,'disconnected mark')
        expected=3 if name in ('a-box-cat','c-coil-cat') else 2
        assert len(holes)==expected,(name,scale,'closed negative spaces changed')
        eye_centers={
            'a-box-cat': ((48,56),(80,56)),
            'b-side-eye': ((44,68),(84,68)),
            'c-coil-cat': ((68,44),(92,44)),
            'd-sitting-cat': ((44,44),(68,44)),
        }[name]
        eye_regions=[]
        for cx,cy in eye_centers:
            sample=(int(cx*scale/8),int(cy*scale/8))
            found=[i for i,hole in enumerate(holes) if sample in hole]
            assert len(found)==1,(name,scale,'eye must be an enclosed clear aperture')
            assert a.getpixel(sample)==0,(name,scale,'eye center must be fully transparent')
            eye_regions.append(found[0])
        assert len(set(eye_regions))==2,(name,scale,'eyes merged')
        assert all(a.getpixel((x,y))==0 for x in range(n) for y in range(n) if x in (0,n-1) or y in (0,n-1)),(name,'clipped')
print('PIXEL PASS: all template eyes, box aperture and tail seam remain open at 1x/2x; no edge clipping.')
