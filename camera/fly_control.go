// Copyright 2016 The G3N Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package camera

import (
	"fmt"

	"github.com/g3n/engine/core"
	"github.com/g3n/engine/gui"
	"github.com/g3n/engine/math32"
	"github.com/g3n/engine/window"
	"github.com/go-gl/glfw/v3.3/glfw"
)

// Notes:
// fly camera allows user to move camera in "FPS" or "flight sim" style
// movement.
// - translation along 3 axes: forward/backward, up/down, left/right (strafe)
// - rotation about 3 axes: roll, pitch, yaw
// thus requires at least 12 inputs.
// will also allow "zoom" (change fov), so +2 inputs.
//
// options
// - look-spring - returns pitch to 0 deg when moving forward/backward
// - mouse-look - captures cursor and makes mouse movements into pitch and yaw.
// - invert pitch - inverts control of pitch so that normal "up" pitched "down", viceversa
// - movements speeds
// - buttons/etc
// - rotation constraints
//
// internally, "right" will be +X, "up" will be +Y, and "forward" will be -Z
// a right handed coordinate system mirroring OpenGL's camera conventions.

type FlyMovement int

const (
	Forward FlyMovement = iota
	Backward
	Right
	Left
	Up
	Down
	YawRight
	YawLeft
	PitchUp
	PitchDown
	RollRight
	RollLeft
	ZoomOut
	ZoomIn
)

type MouseMotion int

const (
	MouseNone        MouseMotion = iota
	MouseRight                   // +X
	MouseLeft                    // -X
	MouseDown                    // +Y
	MouseUp                      // -Y
	MouseScrollRight             // +X
	MouseScrollLeft              // -X
	MouseScrollDown              // +Y
	MouseScrollUp                // -Y
)

const MouseButtonNone window.MouseButton = -1

type MouseGuesture struct {
	Motion MouseMotion
	Button window.MouseButton
}

const degrees = math32.Pi / 180.0
const radians = 1.0 / degrees

type FlyControl struct {
	core.Dispatcher         // Embedded event dispatcher
	cam             *Camera // Controlled camera

	position math32.Vector3 // camera position in the world
	rotation math32.Vector3 // camera yaw, pitch, and roll (radians)
	forward  math32.Vector3 // forward direction of camera
	up       math32.Vector3 // up direction of camera
	upWorld  math32.Vector3 // The world up direction

	mouseIsCaptured  bool
	mousePrevPos     math32.Vector2
	mouseBtnState    uint32 // bitfield. btn0 == 2, btn1 == 4, etc
	mouseSensitivity float32

	// Constraints map.
	// Translation movements aren't used.
	// Angular contraints are in radians.
	Constraints map[FlyMovement]float32

	// Speeds map.
	// Translation movements in units/event,
	// angular movement and zoom in radians/event.
	Speeds map[FlyMovement]float32

	// Keys map.
	Keys map[FlyMovement]window.Key

	// Mouse map.
	Mouse map[FlyMovement]MouseGuesture
}

func NewFlyControl(cam *Camera, target, worldUp *math32.Vector3) *FlyControl {

	fc := new(FlyControl)

	fc.cam = cam
	fc.position = cam.Position()
	fc.Reorient(target, worldUp)

	// TODO: speeds, keys, constraints need to be made into options
	fc.Speeds = map[FlyMovement]float32{
		Forward:   1,
		Backward:  -1,
		Right:     1,
		Left:      -1,
		Up:        1,
		Down:      -1,
		YawRight:  0.1,
		YawLeft:   -0.1,
		PitchUp:   0.1,
		PitchDown: -0.1,
		RollRight: 0.1,
		RollLeft:  -0.1,
		ZoomOut:   0.1,
		ZoomIn:    -0.1,
	}

	fc.Keys = map[FlyMovement]window.Key{
		Forward:   window.KeyUp,
		Backward:  window.KeyDown,
		Right:     window.KeyRight,
		Left:      window.KeyLeft,
		Up:        window.KeyPageUp,
		Down:      window.KeyPageDown,
		YawRight:  window.KeyD,
		YawLeft:   window.KeyA,
		PitchUp:   window.KeyW,
		PitchDown: window.KeyS,
		RollRight: window.KeyE,
		RollLeft:  window.KeyQ,
		ZoomOut:   window.KeyMinus,
		ZoomIn:    window.KeyEqual,
	}

	fc.Constraints = map[FlyMovement]float32{
		// YawRight:  45 * degrees,
		// YawLeft:   -90 * degrees,
		PitchUp:   90 * degrees,
		PitchDown: -90 * degrees,
		RollRight: 45 * degrees,
		RollLeft:  -45 * degrees,
		ZoomOut:   100.0 * degrees,
		ZoomIn:    1.0 * degrees,
	}

	fc.mouseIsCaptured = false
	fc.mousePrevPos = math32.Vector2{math32.NaN(), math32.NaN()}
	fc.mouseBtnState = 0
	fc.mouseSensitivity = 0.5

	fc.Mouse = map[FlyMovement]MouseGuesture{
		YawRight:  {Motion: MouseRight, Button: MouseButtonNone},
		YawLeft:   {Motion: MouseLeft, Button: MouseButtonNone},
		PitchUp:   {Motion: MouseUp, Button: MouseButtonNone},
		PitchDown: {Motion: MouseDown, Button: MouseButtonNone},
		ZoomIn:    {Motion: MouseScrollUp, Button: window.MouseButtonRight},
		ZoomOut:   {Motion: MouseScrollDown, Button: window.MouseButtonRight},
	}

	// Subscribe to events
	// fc.Dispatcher.Initialize()
	// copied from orbit_control.go
	gui.Manager().SubscribeID(window.OnMouseUp, &fc, fc.onMouse)
	gui.Manager().SubscribeID(window.OnMouseDown, &fc, fc.onMouse)
	gui.Manager().SubscribeID(window.OnScroll, &fc, fc.onScroll)
	gui.Manager().SubscribeID(window.OnCursor, &fc, fc.onCursor)

	gui.Manager().SubscribeID(window.OnKeyDown, &fc, fc.onKey)
	gui.Manager().SubscribeID(window.OnKeyRepeat, &fc, fc.onKey)
	// gui.Manager().SubscribeID(window.OnKeyUp, &fc, fc.onKeyUp)

	return fc
}

// Dispose unsubscribes from all events.
func (fc *FlyControl) Dispose() {
	// copied from orbit_control.go
	gui.Manager().UnsubscribeID(window.OnMouseUp, &fc)
	gui.Manager().UnsubscribeID(window.OnMouseDown, &fc)
	gui.Manager().UnsubscribeID(window.OnScroll, &fc)
	gui.Manager().UnsubscribeID(window.OnCursor, &fc)

	gui.Manager().UnsubscribeID(window.OnKeyDown, &fc)
	gui.Manager().UnsubscribeID(window.OnKeyRepeat, &fc)
	// gui.Manager().UnsubscribeID(window.OnKeyUp, &fc)
}

func (fc *FlyControl) Reposition(position *math32.Vector3) {
	fc.position.Copy(position)
	fc.cam.SetPositionVec(position)
}

func (fc *FlyControl) Reorient(target, worldUp *math32.Vector3) {
	fc.rotation.Set(0, 0, 0) // reset the total rotation
	fc.upWorld.Copy(worldUp) // worldUp might have changed
	fc.forward.Copy(target.Clone().Sub(&fc.position).Normalize())
	right := fc.forward.Clone().Cross(worldUp).Normalize()
	fc.up.Copy(right.Cross(&fc.forward).Normalize())
}

// movement changes

func (fc *FlyControl) Forward(delta float32) {
	deltaDir := fc.forward.Clone().MultiplyScalar(delta)
	fc.position.Add(deltaDir)
	fc.cam.SetPositionVec(&fc.position)
}

func (fc *FlyControl) Right(delta float32) {
	// right direction from forward cross up
	deltaDir := fc.forward.Clone().Cross(&fc.up).MultiplyScalar(delta)
	fc.position.Add(deltaDir)
	fc.cam.SetPositionVec(&fc.position)
}

func (fc *FlyControl) Up(delta float32) {
	deltaDir := fc.up.Clone().MultiplyScalar(delta)
	fc.position.Add(deltaDir)
	fc.cam.SetPositionVec(&fc.position)
}

func (fc *FlyControl) Yaw(delta float32) {
	yaw := fc.rotation.X + delta
	if fc.constraintOk(yaw, YawLeft, YawRight) {
		fc.rotation.X = yaw
		// rotation about up axis
		// because of the right hand coord system, positive rotation about up-axis
		// makes camera appear to yaw left instead of right. thus, delta
		// must be inverted below.
		fc.forward.ApplyAxisAngle(&fc.up, -delta)
		fc.cam.LookAt(fc.position.Clone().Add(&fc.forward), &fc.up)
	}
}

func (fc *FlyControl) Pitch(delta float32) {
	pitch := fc.rotation.Y + delta
	if fc.constraintOk(pitch, PitchDown, PitchUp) {
		fc.rotation.Y = pitch
		// rotation about right axis
		// affects both "forward" and "up" directions
		right := fc.forward.Clone().Cross(&fc.up)
		fc.forward.ApplyAxisAngle(right, delta)
		fc.up.ApplyAxisAngle(right, delta)
		fc.cam.LookAt(fc.position.Clone().Add(&fc.forward), &fc.up)
	}
}

func (fc *FlyControl) Roll(delta float32) {
	roll := fc.rotation.Z + delta
	if fc.constraintOk(roll, RollLeft, RollRight) {
		fc.rotation.Z = roll
		// rotation about forward axis
		fc.up.ApplyAxisAngle(&fc.forward, delta)
		fc.cam.LookAt(fc.position.Clone().Add(&fc.forward), &fc.up)
	}
}

// Field of view changes

func (fc *FlyControl) Zoom(delta float32) {
	newFovRad := fc.cam.Fov()*degrees + delta
	if fc.constraintOk(newFovRad, ZoomIn, ZoomOut) {
		fc.cam.SetFov(newFovRad * radians)
	}
}

func (fc *FlyControl) ScaleZoom(fovScale float32) {
	newFovRad := (fc.cam.Fov() * degrees) * fovScale
	if fc.constraintOk(newFovRad, ZoomIn, ZoomOut) {
		fc.cam.SetFov(newFovRad * radians)
	}
}

// checks if newValue is allowed according to the given low and high
// movement constraints.
func (fc *FlyControl) constraintOk(newValue float32, low, high FlyMovement) bool {
	min, minDefined := fc.Constraints[low]
	max, maxDefined := fc.Constraints[high]
	if minDefined && newValue < min {
		return false
	}
	if maxDefined && newValue > max {
		return false
	}
	return true
}

// apply maps FlyMovements to the appropriate method.
func (fc *FlyControl) apply(movement FlyMovement, delta float32) {
	switch movement {
	case Backward, Forward:
		fc.Forward(delta)
	case Left, Right:
		fc.Right(delta)
	case Up, Down:
		fc.Up(delta)
	case YawLeft, YawRight:
		fc.Yaw(delta)
	case PitchUp, PitchDown:
		fc.Pitch(delta)
	case RollLeft, RollRight:
		fc.Roll(delta)
	case ZoomIn, ZoomOut:
		fc.Zoom(delta)
	}
}

func (fc *FlyControl) enableMouseCapture(enable bool) {
	// NOTE: I think it's not really good here to cast the IWindow to *GlfwWindow.
	// Defeats the purpose of having an interface and also makes the package
	// rely on the glfw package.
	if enable {
		win := window.Get().(*window.GlfwWindow)
		win.SetInputMode(glfw.InputMode(window.CursorInputMode), int(window.CursorDisabled))
		fc.mouseIsCaptured = true
	} else {
		win := window.Get().(*window.GlfwWindow)
		win.SetInputMode(glfw.InputMode(window.CursorInputMode), int(window.CursorNormal))
		fc.mouseIsCaptured = false
	}
}

func (fc *FlyControl) toggleMouseButton(button window.MouseButton) {
	fc.mouseBtnState ^= 1 << (1 + int(button)) // toggle button state bit
}

func (fc *FlyControl) mouseButtonPressed(button window.MouseButton) bool {
	if button == MouseButtonNone && fc.mouseBtnState == 0 {
		return true
	}
	bit := uint32(1 << (1 + uint32(button)))
	return fc.mouseBtnState&bit == bit
}

func (fc *FlyControl) resetMousePrevPosition() {
	fc.mousePrevPos = math32.Vector2{math32.NaN(), math32.NaN()}
}

func (fc *FlyControl) isMousePrevPositionUnset() bool {
	return math32.IsNaN(fc.mousePrevPos.X)
}

// copied from orbit_control.go

// onMouse is called when an OnMouseDown/OnMouseUp event is received.
func (fc *FlyControl) onMouse(evname string, ev interface{}) {
	mev := ev.(*window.MouseEvent)
	fc.toggleMouseButton(mev.Button)
	fmt.Println("mouse buttons:", fc.mouseBtnState)
}

// onCursor is called when an OnCursor event is received.
func (fc *FlyControl) onCursor(evname string, ev interface{}) {

	if !fc.mouseIsCaptured {
		return
	}

	mev := ev.(*window.CursorEvent)
	if fc.isMousePrevPositionUnset() {
		fc.mousePrevPos.X = mev.Xpos
		fc.mousePrevPos.Y = mev.Ypos
	}

	const moderator = 0.5
	dx := (mev.Xpos - fc.mousePrevPos.X) * moderator * fc.mouseSensitivity
	dy := (mev.Ypos - fc.mousePrevPos.Y) * moderator * fc.mouseSensitivity
	fc.mousePrevPos.X = mev.Xpos
	fc.mousePrevPos.Y = mev.Ypos

	mouseX := MouseNone
	mouseY := MouseNone
	if dx != 0 {
		if dx > 0 {
			mouseX = MouseRight
		} else {
			mouseX = MouseLeft
		}
	}
	if dy != 0 {
		if dy > 0 {
			mouseY = MouseDown
		} else {
			mouseY = MouseUp
		}
	}
	// fmt.Printf("mouse delta: %f %f, motion: %d %d\n", dx, dy, mouseX, mouseY)

	for m, g := range fc.Mouse {
		pressed := fc.mouseButtonPressed(g.Button)
		if g.Motion == mouseX && pressed {
			speed := fc.Speeds[m]
			fc.apply(m, math32.Abs(dx)*speed)
		}
		if g.Motion == mouseY && pressed {
			speed := fc.Speeds[m]
			fc.apply(m, math32.Abs(dy)*speed)
		}
	}
}

// onScroll is called when an OnScroll event is received.
func (fc *FlyControl) onScroll(evname string, ev interface{}) {
	// x/y offset appears to always be +-1
	sev := ev.(*window.ScrollEvent)

	scrollX := MouseNone
	scrollY := MouseNone
	if sev.Xoffset != 0 {
		if sev.Xoffset > 0 {
			scrollX = MouseScrollRight
		} else {
			scrollX = MouseScrollLeft
		}
	}
	if sev.Yoffset != 0 {
		if sev.Yoffset > 0 {
			scrollY = MouseScrollUp
		} else {
			scrollY = MouseScrollDown
		}
	}

	for m, g := range fc.Mouse {
		pressed := fc.mouseButtonPressed(g.Button)
		if g.Motion == scrollX && pressed {
			speed := fc.Speeds[m]
			fc.apply(m, math32.Abs(sev.Xoffset)*speed)
		}
		if g.Motion == scrollY && pressed {
			speed := fc.Speeds[m]
			fc.apply(m, math32.Abs(sev.Yoffset)*speed)
		}
	}
	fmt.Println("mouse scroll:", sev.Xoffset, sev.Yoffset, "scroll:", scrollX, scrollY)
}

// onKey is called when an OnKeyDown/OnKeyRepeat event is received.
func (fc *FlyControl) onKey(evname string, ev interface{}) {

	// If keyboard control is disabled ignore event
	// if fc.enabled&OrbitKeys == 0 {
	// 	return
	// }

	kev := ev.(*window.KeyEvent)
	if kev.Key == window.KeySpace {
		fc.enableMouseCapture(!fc.mouseIsCaptured)
		if !fc.mouseIsCaptured {
			fc.resetMousePrevPosition()
		}
		return
	}
	// find which movement the key corresponds to
	var movement FlyMovement = -1
	for m, k := range fc.Keys {
		if k == kev.Key {
			movement = m
			break
		}
	}
	if movement == -1 {
		// the pressed key is not mapped to a camera movement
		return
	}

	delta := fc.Speeds[movement]
	fc.apply(movement, delta)
}

// onKeyUp is called when an OnKeyUp event is received.
// func (fc *FlyControl) onKeyUp(evname string, ev interface{}) {
// }

// winSize returns the window height or width based on the camera reference axis.
func (fc *FlyControl) winSize() float32 {

	width, size := window.Get().GetSize()
	if fc.cam.Axis() == Horizontal {
		size = width
	}
	return float32(size)
}
