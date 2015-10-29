package main

// visualmoo is a silly small program to create images to illustrate
// that using the ECB mode-of-operation is typically a Bad Idea.
//
// Copyright 2015 Maarten Everts

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"log"
	"os"
)

// ECB mode is not in the Golang std library (for good reason!). This implementation is taken from https://codereview.appspot.com/7860047/

type ecb struct {
	b         cipher.Block
	blockSize int
}

func newECB(b cipher.Block) *ecb {
	return &ecb{
		b:         b,
		blockSize: b.BlockSize(),
	}
}

type ecbEncrypter ecb

// NewECBEncrypter returns a BlockMode which encrypts in electronic code book
// mode, using the given Block.
func NewECBEncrypter(b cipher.Block) cipher.BlockMode {
	return (*ecbEncrypter)(newECB(b))
}

func (x *ecbEncrypter) BlockSize() int { return x.blockSize }

func (x *ecbEncrypter) CryptBlocks(dst, src []byte) {
	if len(src)%x.blockSize != 0 {
		panic("crypto/cipher: input not full blocks")
	}
	if len(dst) < len(src) {
		panic("crypto/cipher: output smaller than input")
	}
	for len(src) > 0 {
		x.b.Encrypt(dst, src[:x.blockSize])
		src = src[x.blockSize:]
		dst = dst[x.blockSize:]
	}
}

var (
	skipAlpha = flag.Bool("skipalpha", false, "ignore (skip) the alpha channel")
	mode      = flag.String("mode", "ECB", "mode of operation to use, options are: ECB and CBC")
	fixedKey  = flag.String("key", "random", "Key to use, either 'random' (default) or a hexadecimal string representing 16, 24, or 32 bytes.")
)

func main() {
	fmt.Println("visualmoo - illustrate ECB badness through images.")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] <input image> <output image>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() != 2 {
		flag.Usage()
		os.Exit(1)
	}
	inputFile, outputFile := flag.Arg(0), flag.Arg(1)

	reader, err := os.Open(inputFile)
	if err != nil {
		log.Fatal(err)
	}
	defer reader.Close()

	m, _, err := image.Decode(reader)
	imageData := image.NewRGBA(m.Bounds())
	draw.Draw(imageData, imageData.Rect, m, image.ZP, draw.Src)

	key := make([]byte, 16)

	if *fixedKey == "random" {
		// use a random key
		rand.Read(key)
	} else {
		var err error
		key, err = hex.DecodeString(*fixedKey)
		if err != nil {
			log.Fatal("Error decoding key from commandline: ", err)
		}
	}

	bc, err := aes.NewCipher(key)
	if err != nil {
		log.Fatal("Error setting up block cipher: ", err)
	}
	blockSize := bc.BlockSize()

	var blockMode cipher.BlockMode
	switch *mode {
	case "ECB":
		blockMode = NewECBEncrypter(bc)
	case "CBC":
		// Let's simply use all zero IV
		blockMode = cipher.NewCBCEncrypter(bc, make([]byte, blockSize))
	default:
		log.Fatal("Error: unsupported mode-of-operation.")
	}

	numPixels := len(imageData.Pix) / 4
	imageSize := numPixels * 4
	if *skipAlpha {
		imageSize = numPixels * 3
	}

	// Make sure the size of the input/output buffers are a multiple of the block
	// size
	nBlocks := 1 + ((imageSize - 1) / blockSize)
	inputBuffer := make([]byte, nBlocks*blockSize)
	outputBuffer := make([]byte, nBlocks*blockSize)

	// copy the data
	if *skipAlpha {
		// We skip the alpha channel in the RGBA data
		for i := 0; i < numPixels; i++ {
			copy(inputBuffer[i*3:i*3+3], imageData.Pix[i*4:i*4+3])
		}
	} else {
		// We can simply copy the RGBA data
		copy(inputBuffer[:len(imageData.Pix)], imageData.Pix)
	}

	// now encrypt
	blockMode.CryptBlocks(outputBuffer, inputBuffer)

	// And copy back the data into an image
	if *skipAlpha {
		for i := 0; i < numPixels; i++ {
			copy(imageData.Pix[i*4:i*4+3], outputBuffer[i*3:i*3+3])
		}
	} else {
		copy(imageData.Pix, outputBuffer[:len(imageData.Pix)])
	}

	// Write this mangled image to a file
	writer, err := os.Create(outputFile)
	if err != nil {
		log.Fatal(err)
	}
	defer writer.Close()
	newImage := imageData.SubImage(imageData.Rect)
	png.Encode(writer, newImage)
}
